package cbor

import (
	"errors"
	"io"
	"iter"
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
	// DefaultMaxEncodedBytes.
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

// DefaultDecoderOptions is copied when Unmarshal runs or a decoder is
// constructed.
//
// Deprecated: Mutating a global changes every caller of Unmarshal and
// NewDecoder in the process and races with concurrent decoding. Use a
// DecoderOptions value with its Unmarshal method or NewDecoderWithOptions;
// the zero DecoderOptions selects the same defaults.
var DefaultDecoderOptions = DecoderOptions{
	MaxArrayElements: DefaultMaxValue, MaxMapPairs: DefaultMaxValue,
	MaxStringBytes: DefaultMaxValue, MaxBytes: DefaultMaxValue,
	MaxNestedLevels: DefaultMaxNestedLevels, MaxEncodedBytes: DefaultMaxEncodedBytes,
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

// NewDecoder constructs a decoder with a snapshot of DefaultDecoderOptions.
func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{reader: reader, options: DefaultDecoderOptions}
}

// NewDecoderWithOptions constructs a decoder with independent limits.
func NewDecoderWithOptions(reader io.Reader, options DecoderOptions) *Decoder {
	return &Decoder{reader: reader, options: options}
}

// SetMax sets the four per-container and string limits for this decoder only.
//
// Deprecated: Use NewDecoderWithOptions.
func (decoder *Decoder) SetMax(limit int) {
	decoder.options.MaxArrayElements = limit
	decoder.options.MaxMapPairs = limit
	decoder.options.MaxStringBytes = limit
	decoder.options.MaxBytes = limit
}

// SetMaxArrayElements sets the array element limit.
//
// Deprecated: Use NewDecoderWithOptions.
func (decoder *Decoder) SetMaxArrayElements(limit int) { decoder.options.MaxArrayElements = limit }

// SetMaxMapPairs sets the map pair limit.
//
// Deprecated: Use NewDecoderWithOptions.
func (decoder *Decoder) SetMaxMapPairs(limit int) { decoder.options.MaxMapPairs = limit }

// SetMaxStringBytes sets the text string byte limit.
//
// Deprecated: Use NewDecoderWithOptions.
func (decoder *Decoder) SetMaxStringBytes(limit int) { decoder.options.MaxStringBytes = limit }

// SetMaxBytes sets the byte string length limit.
//
// Deprecated: Use NewDecoderWithOptions.
func (decoder *Decoder) SetMaxBytes(limit int) { decoder.options.MaxBytes = limit }

func (options DecoderOptions) normalized() (DecoderOptions, error) {
	for _, limit := range [...]*int{&options.MaxArrayElements, &options.MaxMapPairs, &options.MaxStringBytes, &options.MaxBytes} {
		if *limit < 0 {
			return options, errors.New("cbor: negative decoder limit")
		}
		if *limit == 0 {
			*limit = DefaultMaxValue
		}
	}
	if options.MaxNestedLevels < 0 || options.MaxNestedLevels > MaxNestedLevelsLimit || options.MaxEncodedBytes < 0 {
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

// Unmarshal decodes exactly one item and rejects trailing data. The destination
// is unchanged on malformed input or a type error, except for side effects of
// application-provided UnmarshalCBOR methods.
func Unmarshal(data []byte, destination any) error {
	return DefaultDecoderOptions.Unmarshal(data, destination)
}

// Unmarshal decodes exactly one item using these options.
func (options DecoderOptions) Unmarshal(data []byte, destination any) error {
	target, err := destinationValue(destination)
	if err != nil {
		return err
	}
	if options, err = options.normalized(); err != nil {
		return err
	}
	state := getState(&options)
	defer putState(state)
	parser := parser{input: data, options: options, items: state.items}
	root, err := parser.parse(0)
	state.items = parser.items
	if err != nil {
		return err
	}
	if len(parser.data) != len(data) {
		return newError(ErrMalformed, len(parser.data), "trailing data")
	}
	state.data = parser.data
	return store(target, state, root)
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
	parser := parser{reader: decoder.reader, options: options, data: decoder.buffer[:0], items: state.items}
	root, err := parser.parse(0)
	state.items = parser.items
	if err == nil {
		state.data = parser.data
		err = store(target, state, root)
	}
	if cap(parser.data) <= maxRetainedBuffer {
		decoder.buffer = parser.data
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

// store decodes into a zero value and replaces the destination only on
// success, so failed decodes leave it unchanged. A destination that is
// already zero, the common case, is decoded in place and reset on failure,
// which saves allocating the replacement.
func store(target reflect.Value, state *decodeState, root int) error {
	destination := target.Elem()
	decode := decoderFor(destination.Type())
	if destination.IsZero() {
		if err := decode(state, destination, root); err != nil {
			destination.SetZero()
			return err
		}
		return nil
	}
	replacement := reflect.New(destination.Type()).Elem()
	if err := decode(state, replacement, root); err != nil {
		return err
	}
	destination.Set(replacement)
	return nil
}
