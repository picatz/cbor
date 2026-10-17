package cbor

import (
	"bytes"
	"errors"
	"io"
	"math"
	"reflect"
	"unicode/utf8"
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
	// what Marshal produces with CoreDeterministicEncoding.
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1
	RequireDeterministic bool
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
	for _, limit := range [...]*int{&options.MaxArrayElements, &options.MaxMapPairs, &options.MaxStringBytes, &options.MaxBytes} {
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

// Unmarshal decodes exactly one item using these limits.
func (options DecoderOptions) Unmarshal(data []byte, destination any) error {
	target, err := destinationValue(destination)
	if err != nil {
		return err
	}
	options, err = options.normalized()
	if err != nil {
		return err
	}
	parser := parser{input: data, options: options}
	root, err := parser.item(0)
	if err != nil {
		return err
	}
	if len(parser.data) != len(data) {
		return newError(ErrMalformed, len(parser.data), "trailing data")
	}
	return store(target, options, parser.data, root)
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
	parser := parser{reader: decoder.reader, options: options, data: decoder.buffer[:0]}
	root, err := parser.item(0)
	if err == nil {
		err = store(target, options, parser.data, root)
	}
	if cap(parser.data) <= maxRetainedBuffer {
		decoder.buffer = parser.data
	}
	return err
}

func destinationValue(destination any) (reflect.Value, error) {
	value := reflect.ValueOf(destination)
	if !value.IsValid() || value.Kind() != reflect.Ptr || value.IsNil() {
		return value, errors.New("cbor: destination must be a non-nil pointer")
	}
	return value, nil
}

// store decodes into a fresh value and replaces the destination only on
// success, so failed decodes leave it unchanged.
func store(target reflect.Value, options DecoderOptions, data []byte, root item) error {
	replacement := reflect.New(target.Elem().Type()).Elem()
	state := decodeState{data: data, unknownFields: options.RejectUnknownFields}
	if err := state.assign(replacement, root); err != nil {
		return err
	}
	target.Elem().Set(replacement)
	return nil
}

// item is a parsed, validated CBOR data item. Scalars are kept in their
// encoded form so parsing does not allocate per value; payload and the item's
// raw encoding are offsets into the parser's buffer.
type item struct {
	major    byte
	minor    byte   // additional information
	argument uint64 // integer value, length, tag number, simple value or float bits
	start    int    // offset of the item's initial byte
	payload  int    // offset of a string's content
	end      int    // offset just past the item
	children []item // array elements, alternating map keys and values, or a tag's content
}

func (value item) isFloat() bool { return value.major == 7 && value.minor >= 25 && value.minor <= 27 }

func (value item) float() float64 {
	switch value.minor {
	case 25:
		return halfFloat(uint16(value.argument))
	case 26:
		return float64(math.Float32frombits(uint32(value.argument)))
	}
	return math.Float64frombits(value.argument)
}

func (value item) isNull() bool { return value.major == 7 && value.minor == 22 }

// parser reads one item from input or, when reader is set, from a stream
// without read-ahead. data holds every byte of the current item consumed so
// far; in slice mode it is a prefix of input and is never copied.
type parser struct {
	reader  io.Reader
	input   []byte
	options DecoderOptions
	data    []byte
}

func (parser *parser) read(count int) ([]byte, error) {
	start := len(parser.data)
	if count < 0 || count > parser.options.MaxEncodedBytes-start {
		return nil, newError(ErrLimit, start, "encoded size limit exceeded")
	}
	if parser.reader == nil {
		if count > len(parser.input)-start {
			if start == 0 && len(parser.input) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		}
		parser.data = parser.input[:start+count]
		return parser.data[start:], nil
	}
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

// smallCapacity bounds preallocation from an untrusted count; larger
// containers grow as their elements are actually read.
func smallCapacity(count uint64) int {
	if count > 16 {
		return 16
	}
	return int(count)
}

func (parser *parser) item(depth int) (result item, err error) {
	start := len(parser.data)
	if depth > parser.options.MaxNestedLevels {
		return result, newError(ErrLimit, start, "nesting limit exceeded")
	}
	header, err := parser.read(1)
	if err != nil {
		return result, err
	}
	result.start = start
	result.major = header[0] >> 5
	result.minor = header[0] & 31
	result.argument = uint64(result.minor)
	switch {
	case result.minor == 31:
		if result.major >= 2 && result.major <= 5 {
			return result, newError(ErrUnsupported, start, "indefinite-length item")
		}
		return result, newError(ErrMalformed, start, "unexpected break or indefinite-length header")
	case result.minor >= 28:
		return result, newError(ErrMalformed, start, "reserved additional information")
	case result.minor >= 24:
		width := 1 << (result.minor - 24)
		encoded, err := parser.read(width)
		if err != nil {
			return result, err
		}
		result.argument = 0
		for _, octet := range encoded {
			result.argument = result.argument<<8 | uint64(octet)
		}
		if result.major != 7 && parser.options.RejectNonMinimal && !minimal(result.argument, width) {
			return result, newError(ErrNotDeterministic, start, "non-minimal argument")
		}
	}
	switch result.major {
	case 1:
		if result.argument > math.MaxInt64 {
			return result, newError(ErrUnsupported, start, "negative integer exceeds int64")
		}
	case 2, 3:
		limit := parser.options.MaxBytes
		if result.major == 3 {
			limit = parser.options.MaxStringBytes
		}
		if result.argument > uint64(limit) {
			return result, newError(ErrLimit, start, "string size limit exceeded")
		}
		result.payload = len(parser.data)
		encoded, err := parser.read(int(result.argument))
		if err != nil {
			return result, err
		}
		if result.major == 3 && !utf8.Valid(encoded) {
			return result, newError(ErrInvalid, result.payload, "invalid UTF-8")
		}
	case 4, 5:
		if err := parser.container(&result, depth); err != nil {
			return result, err
		}
	case 6:
		if parser.options.RejectTags {
			return result, newError(ErrUnsupported, start, "tags rejected by decoder options")
		}
		child, err := parser.item(depth + 1)
		if err != nil {
			return result, err
		}
		result.children = []item{child}
	case 7:
		switch {
		case result.minor == 24 && result.argument < 32:
			return result, newError(ErrMalformed, start, "two-byte simple value below 32")
		case result.isFloat() && parser.options.RequireDeterministic:
			if minor, bits := shortestFloat(result.float()); minor != result.minor || bits != result.argument {
				return result, newError(ErrNotDeterministic, start, "floating-point value not in shortest form")
			}
		}
	}
	result.end = len(parser.data)
	return result, nil
}

func minimal(argument uint64, width int) bool {
	switch width {
	case 1:
		return argument >= 24
	case 2:
		return argument > math.MaxUint8
	case 4:
		return argument > math.MaxUint16
	}
	return argument > math.MaxUint32
}

func (parser *parser) container(result *item, depth int) error {
	limit := parser.options.MaxArrayElements
	if result.major == 5 {
		limit = parser.options.MaxMapPairs
	}
	if result.argument > uint64(limit) {
		return newError(ErrLimit, result.start, "container size limit exceeded")
	}
	count := result.argument
	if result.major == 5 {
		count *= 2
	}
	// Every child occupies at least one byte.
	if count > uint64(parser.options.MaxEncodedBytes-len(parser.data)) {
		return newError(ErrLimit, result.start, "container exceeds encoded size limit")
	}
	result.children = make([]item, 0, smallCapacity(count))
	var keys keySet
	for index := uint64(0); index < count; index++ {
		child, err := parser.item(depth + 1)
		if err != nil {
			return err
		}
		if result.major == 5 && index%2 == 0 {
			if err := keys.add(parser.data, result.children, child); err != nil {
				return err
			}
			if parser.options.RequireDeterministic && index > 0 {
				previous := result.children[index-2]
				if bytes.Compare(parser.data[previous.start:previous.end], parser.data[child.start:child.end]) >= 0 {
					return newError(ErrNotDeterministic, child.start, "map keys not in bytewise lexicographic order")
				}
			}
		}
		result.children = append(result.children, child)
	}
	return nil
}

// keyID identifies a map key in the CBOR data model, independent of argument
// width: integers by value, strings by content, floats by numeric value.
type keyID struct {
	kind     byte // major type, or 8 for floating-point values
	argument uint64
	text     string
}

// keySet detects duplicate map keys. Small maps are checked by linear scan
// without allocating; larger maps switch to a hash set.
type keySet struct{ ids map[keyID]struct{} }

const linearKeyScan = 16

func (keys *keySet) add(data []byte, children []item, key item) error {
	if err := checkKeyType(key); err != nil {
		return err
	}
	pairs := len(children) / 2
	if pairs < linearKeyScan {
		for index := 0; index < len(children); index += 2 {
			if sameKey(data, children[index], key) {
				return newError(ErrInvalid, key.start, "duplicate map key")
			}
		}
		return nil
	}
	if keys.ids == nil {
		keys.ids = make(map[keyID]struct{}, 2*linearKeyScan)
		for index := 0; index < len(children); index += 2 {
			keys.ids[identify(data, children[index])] = struct{}{}
		}
	}
	id := identify(data, key)
	if _, ok := keys.ids[id]; ok {
		return newError(ErrInvalid, key.start, "duplicate map key")
	}
	keys.ids[id] = struct{}{}
	return nil
}

func checkKeyType(key item) error {
	if key.major == 4 || key.major == 5 || key.major == 6 {
		return newError(ErrUnsupported, key.start, "composite map keys unsupported")
	}
	if key.isFloat() && math.IsNaN(key.float()) {
		return newError(ErrUnsupported, key.start, "NaN map keys unsupported")
	}
	return nil
}

func sameKey(data []byte, left, right item) bool {
	if left.isFloat() || right.isFloat() {
		return left.isFloat() && right.isFloat() && left.float() == right.float()
	}
	if left.major != right.major || left.argument != right.argument {
		return false
	}
	if left.major == 2 || left.major == 3 {
		return bytes.Equal(data[left.payload:left.end], data[right.payload:right.end])
	}
	return true
}

func identify(data []byte, key item) keyID {
	switch {
	case key.isFloat():
		number := key.float()
		if number == 0 {
			number = 0 // -0 and +0 compare equal, as with sameKey.
		}
		return keyID{kind: 8, argument: math.Float64bits(number)}
	case key.major == 2 || key.major == 3:
		return keyID{kind: key.major, argument: key.argument, text: string(data[key.payload:key.end])}
	}
	return keyID{kind: key.major, argument: key.argument}
}

// decodeState converts parsed items into Go values. data is the buffer the
// items' offsets refer to; decoded values never alias it.
type decodeState struct {
	data          []byte
	unknownFields bool // reject keys that match no struct field exactly
}

func (state decodeState) raw(value item) []byte { return state.data[value.start:value.end:value.end] }

func (state decodeState) bytes(value item) []byte {
	return append([]byte{}, state.data[value.payload:value.end]...)
}

func (state decodeState) text(value item) string { return string(state.data[value.payload:value.end]) }

func (state decodeState) typeError(value item, kind reflect.Type) error {
	return newError(ErrType, value.start, "item cannot decode into "+kind.String()+" without type mismatch or overflow")
}

// generic converts an item to the natural Go representation used for any.
func (state decodeState) generic(value item) (any, error) {
	switch value.major {
	case 0:
		return value.argument, nil
	case 1:
		return -1 - int64(value.argument), nil
	case 2:
		return state.bytes(value), nil
	case 3:
		return state.text(value), nil
	case 4:
		values := make([]any, len(value.children))
		for index, child := range value.children {
			decoded, err := state.generic(child)
			if err != nil {
				return nil, err
			}
			values[index] = decoded
		}
		return values, nil
	case 5:
		values := make(map[any]any, len(value.children)/2)
		for index := 0; index < len(value.children); index += 2 {
			if value.children[index].major == 2 {
				return nil, newError(ErrType, value.children[index].start, "byte-string map key cannot be represented in map[any]any")
			}
			key, err := state.generic(value.children[index])
			if err != nil {
				return nil, err
			}
			decoded, err := state.generic(value.children[index+1])
			if err != nil {
				return nil, err
			}
			values[key] = decoded
		}
		return values, nil
	case 6:
		decoded, err := state.generic(value.children[0])
		if err != nil {
			return nil, err
		}
		return TaggedValue{Number: value.argument, Value: decoded}, nil
	}
	switch value.minor {
	case 20:
		return false, nil
	case 21:
		return true, nil
	case 22:
		return nil, nil
	case 25, 26, 27:
		return value.float(), nil
	}
	return SimpleValue(value.argument), nil
}

var (
	taggedValueType = reflect.TypeOf(TaggedValue{})
	simpleValueType = reflect.TypeOf(SimpleValue(0))
	unmarshalerType = reflect.TypeOf((*Unmarshaler)(nil)).Elem()
)

func (state decodeState) assign(destination reflect.Value, value item) error {
	if destination.Kind() == reflect.Ptr {
		destination.Set(reflect.New(destination.Type().Elem()))
		if destination.Type().Implements(unmarshalerType) {
			return destination.Interface().(Unmarshaler).UnmarshalCBOR(state.raw(value))
		}
		if value.isNull() {
			destination.Set(reflect.Zero(destination.Type()))
			return nil
		}
		return state.assign(destination.Elem(), value)
	}
	if destination.CanAddr() && reflect.PointerTo(destination.Type()).Implements(unmarshalerType) {
		return destination.Addr().Interface().(Unmarshaler).UnmarshalCBOR(state.raw(value))
	}
	if value.isNull() {
		switch destination.Kind() {
		case reflect.Interface, reflect.Map, reflect.Slice:
			destination.Set(reflect.Zero(destination.Type()))
			return nil
		}
		return newError(ErrType, value.start, "null cannot decode into "+destination.Type().String())
	}
	if destination.Kind() == reflect.Interface {
		decoded, err := state.generic(value)
		if err != nil {
			return err
		}
		reflected := reflect.ValueOf(decoded)
		if !reflected.Type().AssignableTo(destination.Type()) {
			return state.typeError(value, destination.Type())
		}
		destination.Set(reflected)
		return nil
	}
	switch destination.Type() {
	case taggedValueType:
		if value.major != 6 {
			return state.typeError(value, destination.Type())
		}
		decoded, err := state.generic(value)
		if err != nil {
			return err
		}
		destination.Set(reflect.ValueOf(decoded))
		return nil
	case simpleValueType:
		if value.major != 7 || value.isFloat() {
			return state.typeError(value, destination.Type())
		}
		destination.SetInt(int64(value.argument))
		return nil
	}
	switch destination.Kind() {
	case reflect.Bool:
		if value.major == 7 && (value.minor == 20 || value.minor == 21) {
			destination.SetBool(value.minor == 21)
			return nil
		}
	case reflect.String:
		if value.major == 3 {
			destination.SetString(state.text(value))
			return nil
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		var decoded int64
		if value.major == 0 && value.argument <= math.MaxInt64 {
			decoded = int64(value.argument)
		} else if value.major == 1 {
			decoded = -1 - int64(value.argument)
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
		if value.isFloat() {
			if decoded := value.float(); !destination.OverflowFloat(decoded) {
				destination.SetFloat(decoded)
				return nil
			}
			break
		}
		if decoded, ok := exactInteger(value); ok {
			if destination.Kind() == reflect.Float32 && float64(float32(decoded)) != decoded {
				break
			}
			destination.SetFloat(decoded)
			return nil
		}
	case reflect.Slice:
		if value.major == 2 && destination.Type().Elem().Kind() == reflect.Uint8 {
			destination.SetBytes(state.bytes(value))
			return nil
		}
		if value.major != 4 {
			break
		}
		destination.Set(reflect.MakeSlice(destination.Type(), len(value.children), len(value.children)))
		for index, child := range value.children {
			if err := state.assign(destination.Index(index), child); err != nil {
				return err
			}
		}
		return nil
	case reflect.Array:
		if value.major == 2 && destination.Type().Elem().Kind() == reflect.Uint8 && uint64(destination.Len()) == value.argument {
			reflect.Copy(destination, reflect.ValueOf(state.data[value.payload:value.end]))
			return nil
		}
		if value.major != 4 || destination.Len() != len(value.children) {
			break
		}
		for index, child := range value.children {
			if err := state.assign(destination.Index(index), child); err != nil {
				return err
			}
		}
		return nil
	case reflect.Map:
		if value.major == 5 {
			return state.assignMap(destination, value)
		}
	case reflect.Struct:
		if value.major == 5 {
			return state.assignStruct(destination, value)
		}
	}
	return state.typeError(value, destination.Type())
}

// exactInteger converts an integer item to float64 only when the conversion
// is exact. Float-to-integer coercion is deliberately absent.
func exactInteger(value item) (float64, bool) {
	switch value.major {
	case 0:
		decoded := float64(value.argument)
		if decoded >= 0x1p64 || uint64(decoded) != value.argument {
			return 0, false
		}
		return decoded, true
	case 1:
		number := -1 - int64(value.argument)
		decoded := float64(number)
		if decoded < -0x1p63 || decoded >= 0x1p63 || int64(decoded) != number {
			return 0, false
		}
		return decoded, true
	}
	return 0, false
}

func (state decodeState) assignMap(destination reflect.Value, value item) error {
	kind := destination.Type()
	destination.Set(reflect.MakeMapWithSize(kind, len(value.children)/2))
	for index := 0; index < len(value.children); index += 2 {
		key := reflect.New(kind.Key()).Elem()
		entry := reflect.New(kind.Elem()).Elem()
		if err := state.assign(key, value.children[index]); err != nil {
			return err
		}
		if key.Kind() == reflect.Interface && !key.IsNil() && !key.Elem().Type().Comparable() {
			return newError(ErrType, value.children[index].start, "map key cannot be represented in Go")
		}
		if err := state.assign(entry, value.children[index+1]); err != nil {
			return err
		}
		if destination.MapIndex(key).IsValid() {
			return newError(ErrInvalid, value.children[index].start, "duplicate destination map key")
		}
		destination.SetMapIndex(key, entry)
	}
	return nil
}

func (state decodeState) assignStruct(destination reflect.Value, value item) error {
	fields, err := fieldsFor(destination.Type())
	if err != nil {
		return err
	}
	var assigned []bool
	for index := 0; index < len(value.children); index += 2 {
		key := value.children[index]
		selected := -1
		for position := range fields {
			if fields[position].matches(state.data, key) {
				selected = position
				break
			}
		}
		if selected < 0 && state.unknownFields {
			return newError(ErrType, key.start, "unknown struct field")
		}
		if selected < 0 && key.major == 3 {
			name := state.data[key.payload:key.end]
			for position := range fields {
				if fields[position].keyKind == 3 && bytes.EqualFold([]byte(fields[position].name), name) {
					if selected >= 0 {
						return newError(ErrType, key.start, "ambiguous struct field")
					}
					selected = position
				}
			}
		}
		if selected < 0 {
			continue
		}
		if assigned == nil {
			assigned = make([]bool, len(fields))
		}
		if assigned[selected] {
			return newError(ErrInvalid, key.start, "duplicate destination struct field")
		}
		assigned[selected] = true
		if err := state.assign(destination.Field(fields[selected].index), value.children[index+1]); err != nil {
			return err
		}
	}
	return nil
}

func (field *structField) matches(data []byte, key item) bool {
	if key.major != field.keyKind {
		return false
	}
	if field.keyKind == 3 {
		return string(data[key.payload:key.end]) == field.name
	}
	return key.argument == field.number
}

// structField is cached per struct type and holds no reference to decoded or
// encoded values.
type structField struct {
	index      int
	keyKind    byte   // 0 (unsigned), 1 (negative) or 3 (text)
	number     uint64 // CBOR argument for integer keys
	name       string // text key, or the field name for integer keys
	encodedKey []byte // preferred encoding of the key
	omitEmpty  bool
}
