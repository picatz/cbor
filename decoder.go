package cbor

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Unmarshaler decodes one validated CBOR item. Implementations must copy data
// they retain after returning.
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

// DecoderOptions bounds each item. Zero limits select defaults; negative limits
// are invalid. Indefinite lengths and duplicate keys are always rejected.
type DecoderOptions struct {
	MaxArrayElements int
	MaxMapPairs      int
	MaxStringBytes   int
	MaxBytes         int
	MaxNestedLevels  int
	MaxEncodedBytes  int
	RejectTags       bool
	RejectNonMinimal bool
}

// DefaultDecoderOptions is copied when a decoder is constructed. Configure it
// before starting concurrent work; prefer NewDecoderWithOptions for isolation.
var DefaultDecoderOptions = DecoderOptions{
	MaxArrayElements: DefaultMaxValue, MaxMapPairs: DefaultMaxValue,
	MaxStringBytes: DefaultMaxValue, MaxBytes: DefaultMaxValue,
	MaxNestedLevels: 64, MaxEncodedBytes: 1 << 20,
}

// Decoder reads one definite-length CBOR item at a time without read-ahead.
// A decoder is not safe for concurrent use. After a decoding error, its stream
// position is unspecified. Successful decoding replaces the destination value.
type Decoder struct {
	reader  io.Reader
	options DecoderOptions
}

// NewDecoder constructs a decoder with a snapshot of DefaultDecoderOptions.
func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{reader: reader, options: DefaultDecoderOptions}
}

// NewDecoderWithOptions constructs a decoder with independent limits.
func NewDecoderWithOptions(reader io.Reader, options DecoderOptions) *Decoder {
	return &Decoder{reader: reader, options: options}
}

// SetMax sets the four per-container and string limits for this decoder only.
func (decoder *Decoder) SetMax(limit int) {
	decoder.options.MaxArrayElements = limit
	decoder.options.MaxMapPairs = limit
	decoder.options.MaxStringBytes = limit
	decoder.options.MaxBytes = limit
}

// SetMaxArrayElements sets the array element limit.
func (decoder *Decoder) SetMaxArrayElements(limit int) { decoder.options.MaxArrayElements = limit }

// SetMaxMapPairs sets the map pair limit.
func (decoder *Decoder) SetMaxMapPairs(limit int) { decoder.options.MaxMapPairs = limit }

// SetMaxStringBytes sets the text string byte limit.
func (decoder *Decoder) SetMaxStringBytes(limit int) { decoder.options.MaxStringBytes = limit }

// SetMaxBytes sets the byte string length limit.
func (decoder *Decoder) SetMaxBytes(limit int) { decoder.options.MaxBytes = limit }

func (options DecoderOptions) normalized() (DecoderOptions, error) {
	for _, limit := range []*int{&options.MaxArrayElements, &options.MaxMapPairs, &options.MaxStringBytes, &options.MaxBytes} {
		if *limit < 0 {
			return options, errors.New("cbor: negative decoder limit")
		}
		if *limit == 0 {
			*limit = DefaultMaxValue
		}
	}
	if options.MaxNestedLevels < 0 || options.MaxNestedLevels > 256 || options.MaxEncodedBytes < 0 {
		return options, errors.New("cbor: invalid decoder limit")
	}
	if options.MaxNestedLevels == 0 {
		options.MaxNestedLevels = 64
	}
	if options.MaxEncodedBytes == 0 {
		options.MaxEncodedBytes = 1 << 20
	}
	return options, nil
}

// Unmarshal decodes exactly one item and rejects trailing data. The destination
// is unchanged on malformed input or a type error, except for side effects of
// application-provided UnmarshalCBOR methods.
func Unmarshal(data []byte, destination any) error {
	return DefaultDecoderOptions.Unmarshal(data, destination)
}

// Unmarshal decodes exactly one item using these limits.
func (options DecoderOptions) Unmarshal(data []byte, destination any) error {
	reader := bytes.NewReader(data)
	decoder := NewDecoderWithOptions(reader, options)
	return decoder.decode(destination, func() error {
		if reader.Len() != 0 {
			return errors.New("cbor: trailing data")
		}
		return nil
	})
}

// Decode reads one item. Clean stream exhaustion returns io.EOF; truncated
// items return io.ErrUnexpectedEOF. No subsequent item is consumed.
func (decoder *Decoder) Decode(destination any) error { return decoder.decode(destination, nil) }

func (decoder *Decoder) decode(destination any, complete func() error) error {
	value := reflect.ValueOf(destination)
	if !value.IsValid() || value.Kind() != reflect.Ptr || value.IsNil() {
		return errors.New("cbor: destination must be a non-nil pointer")
	}
	if decoder.reader == nil {
		return errors.New("cbor: nil reader")
	}
	options, err := decoder.options.normalized()
	if err != nil {
		return err
	}
	parser := parser{reader: decoder.reader, options: options}
	item, err := parser.item(0)
	if err != nil {
		return err
	}
	if complete != nil {
		if err := complete(); err != nil {
			return err
		}
	}
	replacement := reflect.New(value.Elem().Type()).Elem()
	if err := assign(replacement, item); err != nil {
		return err
	}
	value.Elem().Set(replacement)
	return nil
}

type item struct {
	major    byte
	argument uint64
	value    any
	children []item
	raw      []byte
}

type parser struct {
	reader  io.Reader
	options DecoderOptions
	data    []byte
}

func (parser *parser) read(count int) ([]byte, error) {
	if count < 0 || count > parser.options.MaxEncodedBytes-len(parser.data) {
		return nil, errors.New("cbor: encoded size limit exceeded")
	}
	start := len(parser.data)
	parser.data = append(parser.data, make([]byte, count)...)
	read, err := io.ReadFull(parser.reader, parser.data[start:])
	if err != nil {
		parser.data = parser.data[:start+read]
		if err == io.EOF && start > 0 {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	return parser.data[start:], nil
}

func (parser *parser) item(depth int) (result item, err error) {
	if depth > parser.options.MaxNestedLevels {
		return result, errors.New("cbor: nesting limit exceeded")
	}
	start := len(parser.data)
	header, err := parser.read(1)
	if err != nil {
		return result, err
	}
	initial := header[0]
	result.major = initial >> 5
	additional := initial & 31
	result.argument = uint64(additional)
	if additional >= 28 {
		return result, errors.New("cbor: reserved or indefinite additional information")
	}
	if additional >= 24 {
		width := 1 << (additional - 24)
		encoded, err := parser.read(width)
		if err != nil {
			return result, err
		}
		result.argument = 0
		for _, octet := range encoded {
			result.argument = result.argument<<8 | uint64(octet)
		}
		if result.major != 7 && parser.options.RejectNonMinimal {
			if result.argument < 24 || (width > 1 && result.argument <= 255) || (width > 2 && result.argument <= 65535) || (width > 4 && result.argument <= math.MaxUint32) {
				return result, errors.New("cbor: non-minimal argument")
			}
		}
	}
	switch result.major {
	case 0:
		result.value = result.argument
	case 1:
		if result.argument > math.MaxInt64 {
			return result, errors.New("cbor: negative integer exceeds int64")
		}
		result.value = -1 - int64(result.argument)
	case 2, 3:
		limit := parser.options.MaxBytes
		if result.major == 3 {
			limit = parser.options.MaxStringBytes
		}
		if result.argument > uint64(limit) {
			return result, errors.New("cbor: string size limit exceeded")
		}
		encoded, err := parser.read(int(result.argument))
		if err != nil {
			return result, err
		}
		if result.major == 2 {
			result.value = append([]byte{}, encoded...)
		} else {
			if !utf8.Valid(encoded) {
				return result, errors.New("cbor: invalid UTF-8")
			}
			result.value = string(encoded)
		}
	case 4, 5:
		limit := parser.options.MaxArrayElements
		if result.major == 5 {
			limit = parser.options.MaxMapPairs
		}
		if result.argument > uint64(limit) {
			return result, errors.New("cbor: container size limit exceeded")
		}
		count := result.argument
		if result.major == 5 {
			count *= 2
		}
		if count > uint64(parser.options.MaxEncodedBytes-len(parser.data)) {
			return result, errors.New("cbor: container exceeds encoded size limit")
		}
		seen := make(map[any]bool)
		for index := uint64(0); index < count; index++ {
			child, err := parser.item(depth + 1)
			if err != nil {
				return result, err
			}
			if result.major == 5 && index%2 == 0 {
				key, err := comparableKey(child)
				if err != nil {
					return result, err
				}
				if seen[key] {
					return result, errors.New("cbor: duplicate map key")
				}
				seen[key] = true
			}
			result.children = append(result.children, child)
		}
	case 6:
		if parser.options.RejectTags {
			return result, errors.New("cbor: tags forbidden")
		}
		child, err := parser.item(depth + 1)
		if err != nil {
			return result, err
		}
		result.children = []item{child}
	case 7:
		switch additional {
		case 20:
			result.value = false
		case 21:
			result.value = true
		case 22:
			result.value = nil
		case 23:
			result.value = SimpleValueUndefined
		case 25:
			result.value = halfFloat(uint16(result.argument))
		case 26:
			result.value = float64(math.Float32frombits(uint32(result.argument)))
		case 27:
			result.value = math.Float64frombits(result.argument)
		default:
			return result, errors.New("cbor: unsupported simple value")
		}
	}
	result.raw = parser.data[start:len(parser.data):len(parser.data)]
	return result, nil
}

func halfFloat(bits uint16) float64 {
	exponent, fraction := (bits>>10)&31, bits&1023
	var value float64
	switch exponent {
	case 0:
		value = math.Ldexp(float64(fraction), -24)
	case 31:
		if fraction == 0 {
			value = math.Inf(1)
		} else {
			value = math.NaN()
		}
	default:
		value = math.Ldexp(float64(fraction+1024), int(exponent)-25)
	}
	if bits&0x8000 != 0 {
		value = -value
	}
	return value
}

func comparableKey(value item) (any, error) {
	if value.major == 2 {
		return struct{ bytes string }{string(value.value.([]byte))}, nil
	}
	if value.major == 4 || value.major == 5 || value.major == 6 {
		return nil, errors.New("cbor: composite map keys unsupported")
	}
	if number, ok := value.value.(float64); ok && math.IsNaN(number) {
		return nil, errors.New("cbor: NaN map keys unsupported")
	}
	return value.value, nil
}

func generic(value item) (any, error) {
	switch value.major {
	case 4:
		values := make([]any, len(value.children))
		for index, child := range value.children {
			decoded, err := generic(child)
			if err != nil {
				return nil, err
			}
			values[index] = decoded
		}
		return values, nil
	case 5:
		values := make(map[any]any, len(value.children)/2)
		for index := 0; index < len(value.children); index += 2 {
			key, err := generic(value.children[index])
			if err != nil {
				return nil, err
			}
			if key != nil && !reflect.TypeOf(key).Comparable() {
				return nil, errors.New("cbor: map key cannot be represented in Go")
			}
			decoded, err := generic(value.children[index+1])
			if err != nil {
				return nil, err
			}
			values[key] = decoded
		}
		return values, nil
	case 6:
		decoded, err := generic(value.children[0])
		return TaggedValue{Number: value.argument, Value: decoded}, err
	default:
		return value.value, nil
	}
}

func assign(destination reflect.Value, value item) error {
	if destination.Kind() == reflect.Ptr {
		destination.Set(reflect.New(destination.Type().Elem()))
		if decoder, ok := destination.Interface().(Unmarshaler); ok {
			return decoder.UnmarshalCBOR(value.raw)
		}
		if value.major == 7 && value.value == nil {
			destination.Set(reflect.Zero(destination.Type()))
			return nil
		}
		return assign(destination.Elem(), value)
	}
	if destination.CanAddr() {
		if decoder, ok := destination.Addr().Interface().(Unmarshaler); ok {
			return decoder.UnmarshalCBOR(value.raw)
		}
	}
	if value.major == 7 && value.value == nil {
		switch destination.Kind() {
		case reflect.Interface, reflect.Map, reflect.Slice:
			destination.Set(reflect.Zero(destination.Type()))
			return nil
		}
		return fmt.Errorf("cbor: null cannot decode into %s", destination.Type())
	}
	if destination.Kind() == reflect.Interface {
		decoded, err := generic(value)
		if err != nil {
			return err
		}
		reflected := reflect.ValueOf(decoded)
		if !reflected.Type().AssignableTo(destination.Type()) {
			return fmt.Errorf("cbor: value cannot decode into %s", destination.Type())
		}
		destination.Set(reflected)
		return nil
	}
	if destination.Type() == reflect.TypeOf(TaggedValue{}) && value.major == 6 {
		decoded, err := generic(value)
		if err != nil {
			return err
		}
		destination.Set(reflect.ValueOf(decoded))
		return nil
	}
	switch destination.Kind() {
	case reflect.Bool:
		if decoded, ok := value.value.(bool); ok {
			destination.SetBool(decoded)
			return nil
		}
	case reflect.String:
		if value.major == 3 {
			destination.SetString(value.value.(string))
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var decoded int64
		if value.major == 0 && value.argument <= math.MaxInt64 {
			decoded = int64(value.argument)
		} else if value.major == 1 {
			decoded = value.value.(int64)
		} else {
			break
		}
		if !destination.OverflowInt(decoded) {
			destination.SetInt(decoded)
			return nil
		}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if value.major == 0 && !destination.OverflowUint(value.argument) {
			destination.SetUint(value.argument)
			return nil
		}
	case reflect.Float32, reflect.Float64:
		if value.major == 0 || value.major == 1 {
			var decoded float64
			if value.major == 0 {
				decoded = float64(value.argument)
				if decoded >= 18446744073709551616.0 || uint64(decoded) != value.argument {
					break
				}
			} else {
				number := value.value.(int64)
				decoded = float64(number)
				if decoded < -9223372036854775808.0 || decoded >= 9223372036854775808.0 || int64(decoded) != number {
					break
				}
			}
			if destination.Kind() == reflect.Float32 && float64(float32(decoded)) != decoded {
				break
			}
			destination.SetFloat(decoded)
			return nil
		}
		if decoded, ok := value.value.(float64); ok && !destination.OverflowFloat(decoded) {
			destination.SetFloat(decoded)
			return nil
		}
	case reflect.Slice:
		if value.major == 2 && destination.Type().Elem().Kind() == reflect.Uint8 {
			destination.SetBytes(append([]byte{}, value.value.([]byte)...))
			return nil
		}
		if value.major != 4 {
			break
		}
		destination.Set(reflect.MakeSlice(destination.Type(), len(value.children), len(value.children)))
		for index, child := range value.children {
			if err := assign(destination.Index(index), child); err != nil {
				return err
			}
		}
		return nil
	case reflect.Array:
		if value.major != 4 || destination.Len() != len(value.children) {
			break
		}
		for index, child := range value.children {
			if err := assign(destination.Index(index), child); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if value.major != 5 {
			break
		}
		destination.Set(reflect.MakeMapWithSize(destination.Type(), len(value.children)/2))
		for index := 0; index < len(value.children); index += 2 {
			key := reflect.New(destination.Type().Key()).Elem()
			entry := reflect.New(destination.Type().Elem()).Elem()
			if err := assign(key, value.children[index]); err != nil {
				return err
			}
			if key.Kind() == reflect.Interface && !key.IsNil() && !key.Elem().Type().Comparable() {
				return errors.New("cbor: map key cannot be represented in Go")
			}
			if err := assign(entry, value.children[index+1]); err != nil {
				return err
			}
			if destination.MapIndex(key).IsValid() {
				return errors.New("cbor: duplicate destination map key")
			}
			destination.SetMapIndex(key, entry)
		}
		return nil
	case reflect.Struct:
		if value.major != 5 {
			break
		}
		fields, err := fieldsFor(destination.Type())
		if err != nil {
			return err
		}
		assigned := make(map[int]bool)
		for index := 0; index < len(value.children); index += 2 {
			key := value.children[index]
			selected := -1
			for _, field := range fields {
				if key.value == field.key {
					selected = field.index
					break
				}
			}
			if selected < 0 && key.major == 3 {
				for _, field := range fields {
					if name, ok := field.key.(string); ok && strings.EqualFold(name, key.value.(string)) {
						if selected >= 0 {
							return errors.New("cbor: ambiguous struct field")
						}
						selected = field.index
					}
				}
			}
			if selected >= 0 {
				if assigned[selected] {
					return errors.New("cbor: duplicate destination struct field")
				}
				assigned[selected] = true
				if err := assign(destination.Field(selected), value.children[index+1]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return fmt.Errorf("cbor: item cannot decode into %s without type mismatch or overflow", destination.Type())
}

type structField struct {
	index     int
	key       any
	omitEmpty bool
}

func fieldsFor(kind reflect.Type) ([]structField, error) {
	var fields []structField
	seen := make(map[any]bool)
	for index := 0; index < kind.NumField(); index++ {
		field := kind.Field(index)
		if field.PkgPath != "" {
			continue
		}
		tag := strings.Split(field.Tag.Get("cbor"), ",")
		if tag[0] == "-" {
			continue
		}
		name := tag[0]
		if name == "" {
			name = field.Name
		}
		metadata := structField{index: index, key: name}
		for _, option := range tag[1:] {
			switch option {
			case "keyasint":
				if strings.HasPrefix(name, "-") {
					number, err := strconv.ParseInt(name, 10, 64)
					if err != nil {
						return nil, errors.New("cbor: invalid integer field key")
					}
					metadata.key = number
					if number == 0 {
						metadata.key = uint64(0)
					}
				} else {
					number, err := strconv.ParseUint(name, 10, 64)
					if err != nil {
						return nil, errors.New("cbor: invalid integer field key")
					}
					metadata.key = number
				}
			case "omitempty":
				metadata.omitEmpty = true
			case "":
			default:
				return nil, errors.New("cbor: unsupported field tag option")
			}
		}
		if seen[metadata.key] {
			return nil, errors.New("cbor: duplicate struct field key")
		}
		seen[metadata.key] = true
		fields = append(fields, metadata)
	}
	return fields, nil
}
