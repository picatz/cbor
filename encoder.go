package cbor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"reflect"
	"sort"
	"unicode/utf8"
)

// SortMode selects the order of encoded map keys. Struct fields are encoded
// as maps and follow the same order.
type SortMode uint8

const (
	// SortLengthFirst orders keys by encoded length, then bytewise. It is the
	// default and matches the "canonical CBOR" ordering of RFC 7049
	// Section 3.9.
	//
	// https://www.rfc-editor.org/rfc/rfc7049.html#section-3.9
	SortLengthFirst SortMode = iota

	// SortBytewiseLexical orders keys bytewise by their encodings, as required
	// by RFC 8949 core deterministic encoding. Because keys use preferred
	// serialization, this is also the CTAP2 canonical CBOR key order (major
	// type, then encoded length, then bytewise).
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1
	// https://fidoalliance.org/specs/fido-v2.1-ps-20210615/fido-client-to-authenticator-protocol-v2.1-ps-errata-20220621.html#ctap2-canonical-cbor-encoding-form
	SortBytewiseLexical
)

// FloatMode selects how floating-point values are encoded.
type FloatMode uint8

const (
	// FloatBinary64 encodes every float32 and float64 value as binary64. It is
	// the default.
	FloatBinary64 FloatMode = iota

	// FloatShortest encodes each value in the shortest of binary16, binary32
	// and binary64 that preserves it exactly, as RFC 8949 preferred
	// serialization requires. Every NaN is encoded as 0xf97e00, so NaN
	// payloads are not preserved.
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.1
	FloatShortest
)

// EncoderOptions configures encoding. The zero value is the default used by
// Marshal and NewEncoder. Every mode writes definite lengths and the shortest
// integer, length and tag headers.
type EncoderOptions struct {
	Sort   SortMode
	Floats FloatMode
}

// CoreDeterministicEncoding returns options that produce RFC 8949 core
// deterministic encoding: preferred serialization, shortest floating-point
// values and bytewise lexicographic map key order. Embedded RawMessage and
// Marshaler output must already be deterministic, or encoding fails with
// ErrNotDeterministic. DecoderOptions.RequireDeterministic accepts exactly
// this encoding.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1
func CoreDeterministicEncoding() EncoderOptions {
	return EncoderOptions{Sort: SortBytewiseLexical, Floats: FloatShortest}
}

func (options EncoderOptions) validate() error {
	if options.Sort > SortBytewiseLexical || options.Floats > FloatShortest {
		return errors.New("cbor: invalid encoder options")
	}
	return nil
}

func (options EncoderOptions) deterministic() bool {
	return options == CoreDeterministicEncoding()
}

// Encoder writes one item per Encode call.
type Encoder struct {
	writer  io.Writer
	options EncoderOptions
}

// NewEncoder constructs an encoder with default options. Each item is bounded
// to 1 MiB and 64 levels.
func NewEncoder(writer io.Writer) *Encoder { return &Encoder{writer: writer} }

// NewEncoderWithOptions constructs an encoder with the given options.
func NewEncoderWithOptions(writer io.Writer, options EncoderOptions) *Encoder {
	return &Encoder{writer: writer, options: options}
}

// Marshal encodes a value with default options into a caller-owned byte
// slice. Map keys use SortLengthFirst and floats use FloatBinary64; use
// CoreDeterministicEncoding for RFC 8949 deterministic output.
func Marshal(value any) ([]byte, error) { return EncoderOptions{}.Marshal(value) }

// Marshal encodes a value with these options into a caller-owned byte slice.
func (options EncoderOptions) Marshal(value any) ([]byte, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	state := encodeState{options: options}
	return state.encode(nil, reflect.ValueOf(value), 0)
}

// Encode validates and buffers one complete item before writing it. A writer
// failure can still leave partial output and is returned to the caller.
func (encoder *Encoder) Encode(value any) error {
	if encoder.writer == nil {
		return errors.New("cbor: nil writer")
	}
	encoded, err := encoder.options.Marshal(value)
	if err != nil {
		return err
	}
	written, err := encoder.writer.Write(encoded)
	if err == nil && written != len(encoded) {
		err = io.ErrShortWrite
	}
	return err
}

const (
	maxEncodedItem  = 1 << 20
	maxEncodedDepth = 64
)

func appendHeader(output []byte, major byte, argument uint64) []byte {
	marker := major << 5
	switch {
	case argument < 24:
		return append(output, marker|byte(argument))
	case argument <= math.MaxUint8:
		return append(output, marker|24, byte(argument))
	case argument <= math.MaxUint16:
		return append(output, marker|25, byte(argument>>8), byte(argument))
	case argument <= math.MaxUint32:
		return append(output, marker|26, byte(argument>>24), byte(argument>>16), byte(argument>>8), byte(argument))
	default:
		var buffer [9]byte
		buffer[0] = marker | 27
		binary.BigEndian.PutUint64(buffer[1:], argument)
		return append(output, buffer[:]...)
	}
}

var (
	marshalerType = reflect.TypeOf((*Marshaler)(nil)).Elem()
	errSizeLimit  = encodeError(ErrLimit, "encoding size limit exceeded")
)

type encodeState struct{ options EncoderOptions }

func (state *encodeState) encode(output []byte, value reflect.Value, depth int) ([]byte, error) {
	if depth > maxEncodedDepth {
		return nil, encodeError(ErrLimit, "encoding nesting limit exceeded")
	}
	if len(output) >= maxEncodedItem {
		return nil, errSizeLimit
	}
	if !value.IsValid() {
		return append(output, 0xf6), nil
	}
	kind := value.Kind()
	if (kind == reflect.Pointer || kind == reflect.Interface) && value.IsNil() {
		return append(output, 0xf6), nil
	}
	valueType := value.Type()
	if valueType.NumMethod() > 0 && valueType.Implements(marshalerType) && value.CanInterface() {
		return state.encodeMarshaler(output, value.Interface().(Marshaler), depth)
	}
	// Implements, not NumMethod, because tagEncoder's method is unexported.
	if kind == reflect.Struct && valueType.Implements(tagEncoderType) && value.CanInterface() {
		number, content := value.Interface().(tagEncoder).cborTag()
		return state.encode(appendHeader(output, 6, number), content, depth+1)
	}
	switch valueType {
	case taggedValueType:
		if !value.CanInterface() {
			break
		}
		tagged := value.Interface().(TaggedValue)
		return state.encode(appendHeader(output, 6, tagged.Number), reflect.ValueOf(tagged.Value), depth+1)
	case simpleValueType:
		return appendSimple(output, SimpleValue(value.Int()))
	}
	switch kind {
	case reflect.Interface:
		// Interfaces add no CBOR nesting, so they do not consume depth.
		return state.encode(output, value.Elem(), depth)
	case reflect.Pointer:
		// Pointers consume depth so that pointer cycles terminate.
		return state.encode(output, value.Elem(), depth+1)
	case reflect.Bool:
		if value.Bool() {
			return append(output, 0xf5), nil
		}
		return append(output, 0xf4), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number := value.Int()
		if number < 0 {
			return appendHeader(output, 1, uint64(-(number + 1))), nil
		}
		return appendHeader(output, 0, uint64(number)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return appendHeader(output, 0, value.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return state.appendFloat(output, value.Float()), nil
	case reflect.String:
		text := value.String()
		if !utf8.ValidString(text) {
			return nil, encodeError(ErrInvalid, "invalid UTF-8")
		}
		if len(text) > maxEncodedItem-len(output)-9 {
			return nil, errSizeLimit
		}
		return append(appendHeader(output, 3, uint64(len(text))), text...), nil
	case reflect.Slice, reflect.Array:
		if kind == reflect.Slice && value.IsNil() {
			return append(output, 0xf6), nil
		}
		if valueType.Elem().Kind() == reflect.Uint8 {
			return appendByteString(output, value)
		}
		if value.Len() > DefaultMaxValue {
			return nil, encodeError(ErrLimit, "array size limit exceeded")
		}
		output = appendHeader(output, 4, uint64(value.Len()))
		for index := 0; index < value.Len(); index++ {
			var err error
			output, err = state.encode(output, value.Index(index), depth+1)
			if err != nil {
				return nil, err
			}
		}
		if len(output) > maxEncodedItem {
			return nil, errSizeLimit
		}
		return output, nil
	case reflect.Map:
		if value.IsNil() {
			return append(output, 0xf6), nil
		}
		return state.encodeMap(output, value, depth)
	case reflect.Struct:
		return state.encodeStruct(output, value, depth)
	}
	return nil, encodeError(ErrUnsupported, "unsupported type "+valueType.String())
}

// appendByteString encodes []byte and byte arrays such as [16]byte as CBOR
// byte strings.
func appendByteString(output []byte, value reflect.Value) ([]byte, error) {
	length := value.Len()
	if length > maxEncodedItem-len(output)-9 {
		return nil, errSizeLimit
	}
	output = appendHeader(output, 2, uint64(length))
	if value.Kind() == reflect.Slice {
		return append(output, value.Bytes()...), nil
	}
	start := len(output)
	output = append(output, make([]byte, length)...)
	reflect.Copy(reflect.ValueOf(output[start:]), value)
	return output, nil
}

func (state *encodeState) encodeMarshaler(output []byte, marshaler Marshaler, depth int) ([]byte, error) {
	encoded, err := marshaler.MarshalCBOR()
	if err != nil {
		return nil, err
	}
	if len(encoded) > maxEncodedItem-len(output) {
		return nil, errSizeLimit
	}
	if err := validateRawAtDepth(encoded, depth, state.options.deterministic()); err != nil {
		return nil, err
	}
	return append(output, encoded...), nil
}

func (state *encodeState) appendFloat(output []byte, number float64) []byte {
	if state.options.Floats == FloatShortest {
		minor, bits := shortestFloat(number)
		switch minor {
		case 25:
			return append(output, 0xf9, byte(bits>>8), byte(bits))
		case 26:
			return append(output, 0xfa, byte(bits>>24), byte(bits>>16), byte(bits>>8), byte(bits))
		}
	}
	var buffer [9]byte
	buffer[0] = 0xfb
	binary.BigEndian.PutUint64(buffer[1:], math.Float64bits(number))
	return append(output, buffer[:]...)
}

// appendSimple encodes the simple values RFC 8949 allows on the wire: 0
// through 23 in the initial byte and 32 through 255 in one extra byte.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.3
func appendSimple(output []byte, simple SimpleValue) ([]byte, error) {
	switch {
	case simple >= 0 && simple < 24:
		return append(output, 0xe0|byte(simple)), nil
	case simple >= 32 && simple <= 255:
		return append(output, 0xf8, byte(simple)), nil
	}
	return nil, encodeError(ErrInvalid, "simple value cannot be encoded")
}

type encodedPair struct {
	key   []byte
	value reflect.Value
}

type pairSorter struct {
	pairs []encodedPair
	mode  SortMode
}

func (sorter pairSorter) Len() int { return len(sorter.pairs) }
func (sorter pairSorter) Swap(i, j int) {
	sorter.pairs[i], sorter.pairs[j] = sorter.pairs[j], sorter.pairs[i]
}
func (sorter pairSorter) Less(i, j int) bool {
	return keyLess(sorter.mode, sorter.pairs[i].key, sorter.pairs[j].key)
}

func keyLess(mode SortMode, left, right []byte) bool {
	if mode == SortLengthFirst && len(left) != len(right) {
		return len(left) < len(right)
	}
	return bytes.Compare(left, right) < 0
}

func (state *encodeState) writePairs(output []byte, pairs []encodedPair, depth int) ([]byte, error) {
	sort.Sort(pairSorter{pairs: pairs, mode: state.options.Sort})
	output = appendHeader(output, 5, uint64(len(pairs)))
	for _, pair := range pairs {
		if len(pair.key) > maxEncodedItem-len(output) {
			return nil, errSizeLimit
		}
		output = append(output, pair.key...)
		var err error
		output, err = state.encode(output, pair.value, depth+1)
		if err != nil {
			return nil, err
		}
	}
	if len(output) > maxEncodedItem {
		return nil, errSizeLimit
	}
	return output, nil
}

func (state *encodeState) encodeStruct(output []byte, value reflect.Value, depth int) ([]byte, error) {
	info, err := structFor(value.Type())
	if err != nil {
		return nil, err
	}
	fields := info.fields
	if info.toArray {
		output = appendHeader(output, 4, uint64(len(fields)))
		for index := range fields {
			output, err = state.encode(output, value.Field(fields[index].index), depth+1)
			if err != nil {
				return nil, err
			}
		}
		if len(output) > maxEncodedItem {
			return nil, errSizeLimit
		}
		return output, nil
	}
	pairs := make([]encodedPair, 0, len(fields))
	for index := range fields {
		entry := value.Field(fields[index].index)
		if fields[index].omit(entry) {
			continue
		}
		pairs = append(pairs, encodedPair{key: fields[index].encodedKey, value: entry})
	}
	return state.writePairs(output, pairs, depth)
}

func (state *encodeState) encodeMap(output []byte, value reflect.Value, depth int) ([]byte, error) {
	if value.Len() > DefaultMaxValue {
		return nil, encodeError(ErrLimit, "map size limit exceeded")
	}
	// Distinct Go keys of these kinds always have distinct CBOR encodings, so
	// only other key types need the data-model duplicate check.
	keyType := value.Type().Key()
	checkKeys := keyType.NumMethod() > 0
	switch keyType.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
	default:
		checkKeys = true
	}
	pairs := make([]encodedPair, 0, value.Len())
	var seen map[keyID]struct{}
	keyBytes := 0
	iterator := value.MapRange()
	for iterator.Next() {
		encoded, err := state.encode(nil, iterator.Key(), depth+1)
		if err != nil {
			return nil, err
		}
		keyBytes += len(encoded)
		if keyBytes > maxEncodedItem {
			return nil, encodeError(ErrLimit, "map size limit exceeded")
		}
		if checkKeys {
			parser := parser{input: encoded, options: rawOptions}
			key, err := parser.item(0)
			if err != nil {
				return nil, err
			}
			if err := checkKeyType(key); err != nil {
				return nil, err
			}
			if seen == nil {
				seen = make(map[keyID]struct{}, value.Len())
			}
			id := identify(encoded, key)
			if _, ok := seen[id]; ok {
				return nil, encodeError(ErrInvalid, "duplicate encoded map key")
			}
			seen[id] = struct{}{}
		}
		pairs = append(pairs, encodedPair{key: encoded, value: iterator.Value()})
	}
	return state.writePairs(output, pairs, depth)
}

func emptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	}
	return value.IsZero()
}
