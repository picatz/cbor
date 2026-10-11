package cbor

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"unicode/utf8"
)

// Encoder writes definite-length CBOR with shortest integer and length headers.
// Map keys are sorted by encoded length and then byte order. Floats use binary64;
// this is not a claim of full canonical or CTAP canonical encoding.
type Encoder struct{ writer io.Writer }

// NewEncoder constructs an encoder. Each item is bounded to 1 MiB and 64 levels.
func NewEncoder(writer io.Writer) *Encoder { return &Encoder{writer: writer} }

// Marshal encodes a value into a caller-owned byte slice.
func Marshal(value any) ([]byte, error) { return encode(nil, reflect.ValueOf(value), 0) }

// Encode validates and buffers one complete item before writing it. A writer
// failure can still leave partial output and is returned to the caller.
func (encoder *Encoder) Encode(value any) error {
	if encoder.writer == nil {
		return errors.New("cbor: nil writer")
	}
	encoded, err := Marshal(value)
	if err != nil {
		return err
	}
	written, err := encoder.writer.Write(encoded)
	if err == nil && written != len(encoded) {
		err = io.ErrShortWrite
	}
	return err
}

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
		output = append(output, marker|27)
		var buffer [8]byte
		binary.BigEndian.PutUint64(buffer[:], argument)
		return append(output, buffer[:]...)
	}
}

func encode(output []byte, value reflect.Value, depth int) ([]byte, error) {
	if depth > 64 {
		return nil, errors.New("cbor: encoding nesting limit exceeded")
	}
	if len(output) >= 1<<20 {
		return nil, errors.New("cbor: encoding size limit exceeded")
	}
	if !value.IsValid() {
		return append(output, 0xf6), nil
	}
	if (value.Kind() == reflect.Ptr || value.Kind() == reflect.Interface) && value.IsNil() {
		return append(output, 0xf6), nil
	}
	if value.CanInterface() {
		if marshaler, ok := value.Interface().(Marshaler); ok {
			encoded, err := marshaler.MarshalCBOR()
			if err != nil {
				return nil, err
			}
			if len(encoded) > 1<<20-len(output) {
				return nil, errors.New("cbor: encoding size limit exceeded")
			}
			if err := validateRawAtDepth(encoded, depth); err != nil {
				return nil, err
			}
			return append(output, encoded...), nil
		}
		if tagged, ok := value.Interface().(TaggedValue); ok {
			return encode(appendHeader(output, 6, tagged.Number), reflect.ValueOf(tagged.Value), depth+1)
		}
		if simple, ok := value.Interface().(SimpleValue); ok {
			switch simple {
			case SimpleValueFalse, SimpleValueTrue, SimpleValueNull, SimpleValueUndefined:
				return append(output, 0xe0|byte(simple)), nil
			}
			return nil, errors.New("cbor: unsupported simple value")
		}
	}
	switch value.Kind() {
	case reflect.Ptr, reflect.Interface:
		return encode(output, value.Elem(), depth+1)
	case reflect.Bool:
		if value.Bool() {
			return append(output, 0xf5), nil
		}
		return append(output, 0xf4), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		number := value.Int()
		if number < 0 {
			output = appendHeader(output, 1, uint64(-(number + 1)))
		} else {
			output = appendHeader(output, 0, uint64(number))
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		output = appendHeader(output, 0, value.Uint())
	case reflect.Float32, reflect.Float64:
		var buffer [8]byte
		binary.BigEndian.PutUint64(buffer[:], math.Float64bits(value.Float()))
		output = append(output, 0xfb)
		output = append(output, buffer[:]...)
	case reflect.String:
		text := value.String()
		if !utf8.ValidString(text) {
			return nil, errors.New("cbor: invalid UTF-8")
		}
		if len(text) > 1<<20-len(output)-9 {
			return nil, errors.New("cbor: encoding size limit exceeded")
		}
		output = appendHeader(output, 3, uint64(len(text)))
		output = append(output, text...)
	case reflect.Slice, reflect.Array:
		if value.Kind() == reflect.Slice && value.IsNil() {
			return append(output, 0xf6), nil
		}
		if value.Kind() == reflect.Slice && value.Type().Elem().Kind() == reflect.Uint8 {
			if value.Len() > 1<<20-len(output)-9 {
				return nil, errors.New("cbor: encoding size limit exceeded")
			}
			output = appendHeader(output, 2, uint64(value.Len()))
			output = append(output, value.Bytes()...)
			break
		}
		if value.Len() > DefaultMaxValue {
			return nil, errors.New("cbor: array size limit exceeded")
		}
		output = appendHeader(output, 4, uint64(value.Len()))
		for index := 0; index < value.Len(); index++ {
			var err error
			output, err = encode(output, value.Index(index), depth+1)
			if err != nil {
				return nil, err
			}
		}
	case reflect.Map, reflect.Struct:
		if value.Kind() == reflect.Map && value.IsNil() {
			return append(output, 0xf6), nil
		}
		return encodeMap(output, value, depth)
	default:
		return nil, fmt.Errorf("cbor: unsupported type %s", value.Type())
	}
	if len(output) > 1<<20 {
		return nil, errors.New("cbor: encoding size limit exceeded")
	}
	return output, nil
}

type encodedPair struct {
	key   []byte
	value reflect.Value
}

func encodeMap(output []byte, value reflect.Value, depth int) ([]byte, error) {
	var pairs []encodedPair
	keyBytes := 0
	seen := make(map[any]bool)
	add := func(key, entry reflect.Value) error {
		encoded, err := encode(nil, key, depth+1)
		if err != nil {
			return err
		}
		options, err := DefaultDecoderOptions.normalized()
		if err != nil {
			return err
		}
		parser := parser{reader: bytes.NewReader(encoded), options: options}
		parsed, err := parser.item(0)
		if err != nil {
			return err
		}
		identity, err := comparableKey(parsed)
		if err != nil {
			return err
		}
		if seen[identity] {
			return errors.New("cbor: duplicate encoded map key")
		}
		seen[identity] = true
		keyBytes += len(encoded)
		if keyBytes > 1<<20 || len(pairs) >= DefaultMaxValue {
			return errors.New("cbor: map size limit exceeded")
		}
		pairs = append(pairs, encodedPair{key: encoded, value: entry})
		return nil
	}
	if value.Kind() == reflect.Map {
		if value.Len() > DefaultMaxValue {
			return nil, errors.New("cbor: map size limit exceeded")
		}
		iterator := value.MapRange()
		for iterator.Next() {
			if err := add(iterator.Key(), iterator.Value()); err != nil {
				return nil, err
			}
		}
	} else {
		fields, err := fieldsFor(value.Type())
		if err != nil {
			return nil, err
		}
		for _, field := range fields {
			entry := value.Field(field.index)
			if field.omitEmpty && emptyValue(entry) {
				continue
			}
			if err := add(reflect.ValueOf(field.key), entry); err != nil {
				return nil, err
			}
		}
	}
	sort.Slice(pairs, func(left, right int) bool {
		if len(pairs[left].key) != len(pairs[right].key) {
			return len(pairs[left].key) < len(pairs[right].key)
		}
		return bytes.Compare(pairs[left].key, pairs[right].key) < 0
	})
	output = appendHeader(output, 5, uint64(len(pairs)))
	for _, pair := range pairs {
		if len(pair.key) > 1<<20-len(output) {
			return nil, errors.New("cbor: encoding size limit exceeded")
		}
		output = append(output, pair.key...)
		var err error
		output, err = encode(output, pair.value, depth+1)
		if err != nil {
			return nil, err
		}
	}
	return output, nil
}

func emptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	}
	return value.IsZero()
}
