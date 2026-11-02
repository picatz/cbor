package cbor

import (
	"errors"
	"io"
	"iter"
	"math"
	"reflect"
)

// Unmarshaler decodes one validated CBOR item. The data may alias the input or
// the decoder's buffer, so implementations must copy data they retain after
// returning.
type Unmarshaler interface{ UnmarshalCBOR([]byte) error }

// Marshaler returns the encoding of exactly one CBOR item.
type Marshaler interface{ MarshalCBOR() ([]byte, error) }

// TaggedValue preserves a tag without assigning application-specific meaning.
type TaggedValue struct {
	Number uint64
	Value  any
}

// DefaultMaxValue is the default per-container and string limit.
const DefaultMaxValue = 10_000

// Default limits selected by zero DecoderOptions fields.
const (
	DefaultMaxNestedLevels = 64
	DefaultMaxEncodedBytes = 1 << 20

	// MaxNestedLevelsLimit is the largest accepted MaxNestedLevels.
	MaxNestedLevelsLimit = 256
)

// DecoderOptions configures decoding, in the style of protobuf's
// UnmarshalOptions: a plain value whose zero value selects the defaults and
// whose methods decode with it. Zero limits select defaults; negative limits
// are invalid. Indefinite lengths and duplicate map keys are always rejected.
//
// Limits are checked before the corresponding allocation or recursion, so
// they bound the work and memory an untrusted item can demand.
type DecoderOptions struct {
	// MaxArrayElements limits the elements of one array. The default is
	// DefaultMaxValue.
	MaxArrayElements int

	// MaxMapPairs limits the key/value pairs of one map. The default is
	// DefaultMaxValue.
	MaxMapPairs int

	// MaxStringBytes limits the length of one text string. The default is
	// DefaultMaxValue.
	MaxStringBytes int

	// MaxBytes limits the length of one byte string. The default is
	// DefaultMaxValue.
	MaxBytes int

	// MaxNestedLevels limits array, map and tag nesting. The default is
	// DefaultMaxNestedLevels and the maximum is MaxNestedLevelsLimit.
	MaxNestedLevels int

	// MaxEncodedBytes limits the encoded size of one item. The default is
	// DefaultMaxEncodedBytes and the maximum is math.MaxInt32. Decoding
	// builds no index of the input, so its memory is that of the decoded
	// value.
	MaxEncodedBytes int

	// RejectTags fails decoding with ErrUnsupported at any tag.
	RejectTags bool

	// RejectUnknownFields fails struct decoding with ErrType when a map key
	// matches no field's key exactly. It also disables the case-insensitive
	// fallback for text keys, so each key has exactly one spelling.
	RejectUnknownFields bool

	// RejectNonMinimal rejects integer, length and tag arguments that are not
	// encoded in their shortest form.
	RejectNonMinimal bool

	// RequireDeterministic accepts only items in RFC 8949 core deterministic
	// encoding: shortest arguments, shortest floating-point values that
	// preserve the value (with every NaN as 0xf97e00), and map keys in
	// bytewise lexicographic order of their encodings. Such input is exactly
	// what Marshal produces with CoreDeterministicEncoding. Bignums decoded
	// into big.Int must also use preferred serialization: no leading zero
	// bytes, and an integer instead of a bignum when one fits.
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1
	RequireDeterministic bool
}

// Decoder reads one definite-length CBOR item at a time without read-ahead.
// A decoder is not safe for concurrent use. After a decoding error, its stream
// position is unspecified. Successful decoding replaces the destination value.
type Decoder struct {
	reader  io.Reader
	options DecoderOptions

	// buffer is the previous item's storage, reused for the next one. Decoded
	// values never alias it, and Unmarshaler implementations must already
	// copy what they retain.
	buffer []byte
}

// maxRetainedBuffer bounds the storage a Decoder keeps after a large item.
const maxRetainedBuffer = 64 << 10

// NewDecoder constructs a decoder with the default options.
func NewDecoder(reader io.Reader) *Decoder { return &Decoder{reader: reader} }

// NewDecoderWithOptions constructs a decoder with independent limits.
func NewDecoderWithOptions(reader io.Reader, options DecoderOptions) *Decoder {
	return &Decoder{reader: reader, options: options}
}

// defaultOptions is the zero DecoderOptions after normalization.
var defaultOptions = DecoderOptions{
	MaxArrayElements: DefaultMaxValue, MaxMapPairs: DefaultMaxValue,
	MaxStringBytes: DefaultMaxValue, MaxBytes: DefaultMaxValue,
	MaxNestedLevels: DefaultMaxNestedLevels, MaxEncodedBytes: DefaultMaxEncodedBytes,
}

func (options DecoderOptions) normalized() (DecoderOptions, error) {
	if options == (DecoderOptions{}) {
		return defaultOptions, nil
	}
	for _, limit := range [...]*int{&options.MaxArrayElements, &options.MaxMapPairs, &options.MaxStringBytes, &options.MaxBytes} {
		if *limit < 0 {
			return options, errors.New("cbor: negative decoder limit")
		}
		if *limit == 0 {
			*limit = DefaultMaxValue
		}
	}
	if options.MaxNestedLevels < 0 || options.MaxNestedLevels > MaxNestedLevelsLimit || options.MaxEncodedBytes < 0 || options.MaxEncodedBytes > math.MaxInt32 {
		return options, errors.New("cbor: invalid decoder limit")
	}
	if options.MaxNestedLevels == 0 {
		options.MaxNestedLevels = DefaultMaxNestedLevels
	}
	if options.MaxEncodedBytes == 0 {
		options.MaxEncodedBytes = DefaultMaxEncodedBytes
	}
	if options.RequireDeterministic {
		options.RejectNonMinimal = true
	}
	return options, nil
}

// Unmarshal decodes exactly one item with the default options and rejects
// trailing data. The destination
// is unchanged on malformed input or a type error, except for side effects of
// application-provided UnmarshalCBOR methods.
func Unmarshal(data []byte, destination any) error {
	return DecoderOptions{}.Unmarshal(data, destination)
}

// Unmarshal decodes exactly one item using these options.
func (options DecoderOptions) Unmarshal(data []byte, destination any) error {
	target, err := destinationValue(destination)
	if err != nil {
		return err
	}
	settings := &defaultOptions
	if options != (DecoderOptions{}) {
		if options, err = options.normalized(); err != nil {
			return err
		}
		settings = &options
	}
	state := getState(settings)
	defer putState(state)
	state.setInput(data)
	return store(target, state)
}

// Decode reads one item. Clean stream exhaustion returns io.EOF; truncated
// items return io.ErrUnexpectedEOF. No subsequent item is consumed.
func (decoder *Decoder) Decode(destination any) error {
	target, err := destinationValue(destination)
	if err != nil {
		return err
	}
	if decoder.reader == nil {
		return errors.New("cbor: nil reader")
	}
	options, err := decoder.options.normalized()
	if err != nil {
		return err
	}
	state := getState(&options)
	defer putState(state)
	// A stream's remaining length is unknown, so read and validate the whole
	// item before decoding it from the buffer; allocations sized from its
	// counts are then bounded by bytes actually received.
	state.reader, state.data = decoder.reader, decoder.buffer[:0]
	var whole item
	if err = state.skip(&whole, 0); err == nil {
		state.reader = nil
		state.setInput(state.data)
		err = store(target, state)
	}
	if cap(state.data) <= maxRetainedBuffer {
		decoder.buffer = state.data
	}
	return err
}

// All returns an iterator over the remaining items of the stream, read as a
// CBOR sequence (RFC 8742). Each RawMessage owns a copy of one item's
// encoding and can be decoded later with Unmarshal. Iteration ends when the
// stream ends cleanly, or after the first error is yielded; a truncated item
// yields io.ErrUnexpectedEOF.
//
// https://www.rfc-editor.org/rfc/rfc8742.html
func (decoder *Decoder) All() iter.Seq2[RawMessage, error] {
	return func(yield func(RawMessage, error) bool) {
		for {
			var message RawMessage
			err := decoder.Decode(&message)
			if err == io.EOF {
				return
			}
			if !yield(message, err) || err != nil {
				return
			}
		}
	}
}

func destinationValue(destination any) (reflect.Value, error) {
	value := reflect.ValueOf(destination)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return value, errors.New("cbor: destination must be a non-nil pointer")
	}
	return value, nil
}

// store reads one item from state and decodes it into a zero value, then
// replaces the destination only on success, so failed decodes leave it
// unchanged. A destination that is already zero, the common case, is decoded
// in place and reset on failure, which saves allocating the replacement.
//
// Decoding and validation are one pass, so UnmarshalCBOR calls are queued
// and run only once the whole item, and in slice mode the absence of
// trailing data, has been validated.
func store(target reflect.Value, state *decodeState) error {
	destination := target.Elem()
	decode := state.decoderFor(destination.Type())
	inPlace := destination.IsZero()
	replacement := destination
	if !inPlace {
		replacement = reflect.New(destination.Type()).Elem()
	}
	if inPlace {
		// A panicking UnmarshalCBOR must not leave a partly decoded value.
		defer func() {
			if recovered := recover(); recovered != nil {
				destination.SetZero()
				panic(recovered)
			}
		}()
	}
	err := func() error {
		var root item
		if err := state.item(&root, 0); err != nil {
			return err
		}
		if err := decode(state, replacement, root, 0); err != nil {
			return err
		}
		if state.reader == nil && state.pos != len(state.data) {
			return newError(ErrMalformed, state.pos, "trailing data")
		}
		return state.flush()
	}()
	if err != nil {
		if inPlace {
			destination.SetZero()
		}
		return err
	}
	if !inPlace {
		destination.Set(replacement)
	}
	return nil
}
