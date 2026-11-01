package cbor

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"strings"
	"sync"
)

// decodeState converts parsed items into Go values. data is the buffer the
// items' offsets refer to; decoded values never alias it.
//
// Every decoderFunc writes into a zero value: the destination is a fresh
// replacement, and containers reset their scratch elements between entries.
type decodeState struct {
	data          []byte
	items         []item
	rejectUnknown bool // reject keys that match no struct field exactly
	deterministic bool // require preferred serialization of bignums
}

var statePool = sync.Pool{New: func() any { return new(decodeState) }}

func getState(options *DecoderOptions) *decodeState {
	state := statePool.Get().(*decodeState)
	state.rejectUnknown = options.RejectUnknownFields
	state.deterministic = options.RequireDeterministic
	return state
}

func putState(state *decodeState) {
	state.data = nil
	if cap(state.items) > maxPooledItems {
		state.items = nil
	}
	state.items = state.items[:0]
	statePool.Put(state)
}

// decoderFunc decodes the item at index into destination, which is
// addressable and holds its type's zero value.
type decoderFunc func(state *decodeState, destination reflect.Value, index int) error

// decoderCache maps a reflect.Type to its decoderFunc.
var decoderCache sync.Map

// decoderFor returns the decoder for a type, building it once. Recursive
// types receive an indirect decoder while their own is under construction.
func decoderFor(kind reflect.Type) decoderFunc {
	if cached, ok := decoderCache.Load(kind); ok {
		return cached.(decoderFunc)
	}
	var (
		ready   sync.WaitGroup
		decoder decoderFunc
	)
	ready.Add(1)
	cached, loaded := decoderCache.LoadOrStore(kind, decoderFunc(func(state *decodeState, destination reflect.Value, index int) error {
		ready.Wait()
		return decoder(state, destination, index)
	}))
	if loaded {
		return cached.(decoderFunc)
	}
	defer func() {
		if decoder == nil {
			// Construction panicked: release any waiters with an error and
			// drop the placeholder, so the cache is not left poisoned.
			decoderCache.CompareAndDelete(kind, cached)
			decoder = func(*decodeState, reflect.Value, int) error {
				return encodeError(ErrUnsupported, "no decoder for "+kind.String())
			}
		}
		ready.Done()
	}()
	built := newDecoder(kind)
	decoder = built
	decoderCache.Store(kind, built)
	return built
}

var (
	anyType         = reflect.TypeFor[any]()
	bigIntType      = reflect.TypeFor[big.Int]()
	rawMessageType  = reflect.TypeFor[RawMessage]()
	simpleValueType = reflect.TypeFor[SimpleValue]()
	taggedValueType = reflect.TypeFor[TaggedValue]()
	unmarshalerType = reflect.TypeFor[Unmarshaler]()
)

func newDecoder(kind reflect.Type) decoderFunc {
	if kind.Kind() == reflect.Pointer && kind.Implements(unmarshalerType) {
		return decodeUnmarshalerPointer
	}
	if kind.Kind() != reflect.Pointer && reflect.PointerTo(kind).Implements(unmarshalerType) {
		if kind == rawMessageType {
			return decodeRawMessage
		}
		return decodeUnmarshaler
	}
	switch kind {
	case taggedValueType:
		return decodeTaggedValue
	case simpleValueType:
		return decodeSimpleValue
	case bigIntType:
		return decodeBigInt
	}
	if kind.Kind() == reflect.Struct && reflect.PointerTo(kind).Implements(tagDecoderType) {
		return newTagDecoder(kind)
	}
	switch kind.Kind() {
	case reflect.Pointer:
		return newPointerDecoder(kind)
	case reflect.Interface:
		if kind == anyType {
			return decodeAny
		}
		return decodeInterface
	case reflect.Bool:
		return decodeBool
	case reflect.String:
		return decodeString
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return decodeInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return decodeUint
	case reflect.Float32, reflect.Float64:
		return decodeFloat
	case reflect.Slice:
		return newSliceDecoder(kind)
	case reflect.Array:
		return newArrayDecoder(kind)
	case reflect.Map:
		return newMapDecoder(kind)
	case reflect.Struct:
		return newStructDecoder(kind)
	}
	return decodeUnsupported
}

func (state *decodeState) raw(index int) []byte {
	value := &state.items[index]
	return state.data[value.start:value.end:value.end]
}

func (state *decodeState) payload(value *item) []byte {
	return state.data[value.payload():value.end]
}

func (state *decodeState) typeError(value *item, kind reflect.Type) error {
	if value.isNull() {
		return newError(ErrType, value.start, "cannot decode null into "+kind.String())
	}
	return newError(ErrType, value.start, "cannot decode "+describe(value)+" into "+kind.String())
}

func (state *decodeState) overflow(value *item, kind reflect.Type) error {
	return newError(ErrType, value.start, describe(value)+" overflows "+kind.String())
}

// describe names an item's kind for error messages.
func describe(value *item) string {
	switch value.major {
	case 0:
		return "unsigned integer"
	case 1:
		return "negative integer"
	case 2:
		return "byte string"
	case 3:
		return "text string"
	case 4:
		return "array"
	case 5:
		return "map"
	case 6:
		return "tag"
	}
	switch {
	case value.minor == 20 || value.minor == 21:
		return "boolean"
	case value.isFloat():
		return "floating-point number"
	}
	return "simple value"
}

func decodeUnsupported(state *decodeState, destination reflect.Value, index int) error {
	return state.typeError(&state.items[index], destination.Type())
}

func decodeUnmarshalerPointer(state *decodeState, destination reflect.Value, index int) error {
	destination.Set(reflect.New(destination.Type().Elem()))
	return destination.Interface().(Unmarshaler).UnmarshalCBOR(state.raw(index))
}

func decodeUnmarshaler(state *decodeState, destination reflect.Value, index int) error {
	return destination.Addr().Interface().(Unmarshaler).UnmarshalCBOR(state.raw(index))
}

func decodeRawMessage(state *decodeState, destination reflect.Value, index int) error {
	destination.SetBytes(bytes.Clone(state.raw(index)))
	return nil
}

func newPointerDecoder(kind reflect.Type) decoderFunc {
	element := kind.Elem()
	decode := decoderFor(element)
	return func(state *decodeState, destination reflect.Value, index int) error {
		if state.items[index].isNull() {
			return nil
		}
		allocated := reflect.New(element)
		if err := decode(state, allocated.Elem(), index); err != nil {
			return err
		}
		destination.Set(allocated)
		return nil
	}
}

func decodeAny(state *decodeState, destination reflect.Value, index int) error {
	if state.items[index].isNull() {
		return nil
	}
	decoded, err := state.generic(index)
	if err != nil {
		return err
	}
	*destination.Addr().Interface().(*any) = decoded
	return nil
}

func decodeInterface(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	if value.isNull() {
		return nil
	}
	decoded, err := state.generic(index)
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

func decodeTaggedValue(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	if value.major != 6 {
		return state.typeError(value, destination.Type())
	}
	decoded, err := state.generic(index)
	if err != nil {
		return err
	}
	*destination.Addr().Interface().(*TaggedValue) = decoded.(TaggedValue)
	return nil
}

func decodeSimpleValue(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	if value.major != 7 || value.isFloat() || value.isNull() {
		return state.typeError(value, destination.Type())
	}
	destination.SetInt(int64(value.argument))
	return nil
}

func newTagDecoder(kind reflect.Type) decoderFunc {
	number, content, ok := tagFields(kind)
	if !ok {
		return func(*decodeState, reflect.Value, int) error {
			return encodeError(ErrUnsupported, "ambiguous or unexported embedded Tag in "+kind.String())
		}
	}
	contentField := kind.FieldByIndex(content)
	decode := decoderFor(contentField.Type)
	return func(state *decodeState, destination reflect.Value, index int) error {
		value := &state.items[index]
		if value.major != 6 {
			return state.typeError(value, destination.Type())
		}
		fieldForDecode(destination, number).SetUint(value.argument)
		return decode(state, fieldForDecode(destination, content), index+1)
	}
}

func decodeBool(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	if value.major == 7 && (value.minor == 20 || value.minor == 21) {
		destination.SetBool(value.minor == 21)
		return nil
	}
	return state.typeError(value, destination.Type())
}

func decodeString(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	if value.major != 3 {
		return state.typeError(value, destination.Type())
	}
	destination.SetString(string(state.payload(value)))
	return nil
}

func decodeInt(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	var decoded int64
	switch {
	case value.major == 0 && value.argument <= math.MaxInt64:
		decoded = int64(value.argument)
	case value.major == 1 && value.argument <= math.MaxInt64:
		decoded = -1 - int64(value.argument)
	case value.major == 0 || value.major == 1:
		return state.overflow(value, destination.Type())
	default:
		return state.typeError(value, destination.Type())
	}
	if destination.OverflowInt(decoded) {
		return state.overflow(value, destination.Type())
	}
	destination.SetInt(decoded)
	return nil
}

func decodeUint(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	switch {
	case value.major == 1:
		return state.overflow(value, destination.Type())
	case value.major != 0:
		return state.typeError(value, destination.Type())
	case destination.OverflowUint(value.argument):
		return state.overflow(value, destination.Type())
	}
	destination.SetUint(value.argument)
	return nil
}

func decodeFloat(state *decodeState, destination reflect.Value, index int) error {
	value := &state.items[index]
	if value.isFloat() {
		decoded := value.float()
		if destination.OverflowFloat(decoded) {
			return state.overflow(value, destination.Type())
		}
		destination.SetFloat(decoded)
		return nil
	}
	decoded, ok := exactInteger(value)
	if !ok || destination.Kind() == reflect.Float32 && float64(float32(decoded)) != decoded {
		return state.typeError(value, destination.Type())
	}
	destination.SetFloat(decoded)
	return nil
}

// exactInteger converts an integer item to float64 only when the conversion
// is exact. Float-to-integer coercion is deliberately absent.
func exactInteger(value *item) (float64, bool) {
	switch value.major {
	case 0:
		decoded := float64(value.argument)
		if decoded >= 0x1p64 || uint64(decoded) != value.argument {
			return 0, false
		}
		return decoded, true
	case 1:
		if value.argument > math.MaxInt64 {
			return 0, false
		}
		number := -1 - int64(value.argument)
		decoded := float64(number)
		if decoded < -0x1p63 || decoded >= 0x1p63 || int64(decoded) != number {
			return 0, false
		}
		return decoded, true
	}
	return 0, false
}

func newSliceDecoder(kind reflect.Type) decoderFunc {
	byteString := kind.Elem().Kind() == reflect.Uint8
	element := decoderFor(kind.Elem())
	return func(state *decodeState, destination reflect.Value, index int) error {
		value := &state.items[index]
		switch {
		case value.isNull():
			return nil
		case byteString && value.major == 2:
			destination.SetBytes(append(make([]byte, 0, value.argument), state.payload(value)...))
			return nil
		case value.major != 4:
			return state.typeError(value, destination.Type())
		}
		count := int(value.argument)
		slice := reflect.MakeSlice(kind, count, count)
		child := index + 1
		for position := range count {
			if err := element(state, slice.Index(position), child); err != nil {
				return err
			}
			child = state.items[child].next
		}
		destination.Set(slice)
		return nil
	}
}

func newArrayDecoder(kind reflect.Type) decoderFunc {
	byteString := kind.Elem().Kind() == reflect.Uint8
	element := decoderFor(kind.Elem())
	length := kind.Len()
	return func(state *decodeState, destination reflect.Value, index int) error {
		value := &state.items[index]
		if byteString && value.major == 2 && value.argument == uint64(length) {
			copy(destination.Bytes(), state.payload(value))
			return nil
		}
		if value.major != 4 || value.argument != uint64(length) {
			return state.typeError(value, destination.Type())
		}
		child := index + 1
		for position := range length {
			if err := element(state, destination.Index(position), child); err != nil {
				return err
			}
			child = state.items[child].next
		}
		return nil
	}
}

func newMapDecoder(kind reflect.Type) decoderFunc {
	keyType, elementType := kind.Key(), kind.Elem()
	decodeKey, decodeElement := decoderFor(keyType), decoderFor(elementType)
	interfaceKey := keyType.Kind() == reflect.Interface
	return func(state *decodeState, destination reflect.Value, index int) error {
		value := &state.items[index]
		if value.isNull() {
			return nil
		}
		if value.major != 5 {
			return state.typeError(value, destination.Type())
		}
		count := int(value.argument)
		result := reflect.MakeMapWithSize(kind, count)
		key, element := reflect.New(keyType).Elem(), reflect.New(elementType).Elem()
		child := index + 1
		for range count {
			if interfaceKey && state.items[child].major == 1 && state.items[child].argument > math.MaxInt64 {
				// A *big.Int key would compare by pointer, not by value.
				return newError(ErrType, state.items[child].start, "integer map key below math.MinInt64 cannot be an interface map key")
			}
			key.SetZero()
			if err := decodeKey(state, key, child); err != nil {
				return err
			}
			if interfaceKey && !key.IsNil() && !key.Elem().Type().Comparable() {
				return newError(ErrType, state.items[child].start, "map key cannot be represented in Go")
			}
			entry := state.items[child].next
			element.SetZero()
			if err := decodeElement(state, element, entry); err != nil {
				return err
			}
			// Distinct CBOR keys can decode to equal Go keys, such as
			// through an Unmarshaler; that is a duplicate in Go terms.
			before := result.Len()
			result.SetMapIndex(key, element)
			if result.Len() == before {
				return newError(ErrInvalid, state.items[child].start, "duplicate destination map key")
			}
			child = state.items[entry].next
		}
		destination.Set(result)
		return nil
	}
}

// fieldSet records which struct fields a map assigned.
type fieldSet struct {
	small uint64
	large []uint64
}

func (set *fieldSet) add(position int) bool {
	if position < 64 && set.large == nil {
		if set.small&(1<<position) != 0 {
			return false
		}
		set.small |= 1 << position
		return true
	}
	if set.large == nil {
		set.large = make([]uint64, position/64+1)
		set.large[0] = set.small
	}
	if word := position / 64; word >= len(set.large) {
		set.large = append(set.large, make([]uint64, word-len(set.large)+1)...)
	}
	word, bit := position/64, uint64(1)<<(position%64)
	if set.large[word]&bit != 0 {
		return false
	}
	set.large[word] |= bit
	return true
}

func (set *fieldSet) has(position int) bool {
	if set.large == nil {
		return position < 64 && set.small&(1<<position) != 0
	}
	word := position / 64
	return word < len(set.large) && set.large[word]&(1<<(position%64)) != 0
}

func newStructDecoder(kind reflect.Type) decoderFunc {
	info, err := structFor(kind)
	if err != nil {
		return func(*decodeState, reflect.Value, int) error { return err }
	}
	fields := info.fields
	decoders := make([]decoderFunc, len(fields))
	for position := range fields {
		decoders[position] = decoderFor(fields[position].typ)
	}
	if info.toArray {
		return func(state *decodeState, destination reflect.Value, index int) error {
			value := &state.items[index]
			if value.major != 4 {
				return state.typeError(value, destination.Type())
			}
			if value.argument != uint64(len(fields)) {
				return newError(ErrType, value.start, "array length does not match the fields of "+destination.Type().String())
			}
			child := index + 1
			for position := range fields {
				if err := decoders[position](state, fieldForDecode(destination, fields[position].index), child); err != nil {
					return err
				}
				child = state.items[child].next
			}
			return nil
		}
	}
	return func(state *decodeState, destination reflect.Value, index int) error {
		value := &state.items[index]
		if value.major != 5 {
			return state.typeError(value, destination.Type())
		}
		var assigned fieldSet
		hint := 0
		child := index + 1
		for range value.argument {
			key := &state.items[child]
			entry := key.next
			position := info.lookup(state.data, key, hint)
			if position < 0 {
				if state.rejectUnknown {
					return newError(ErrType, key.start, "unknown struct field")
				}
				if position = info.fold(state.data, key); position == -2 {
					return newError(ErrType, key.start, "ambiguous struct field")
				}
			}
			if position >= 0 {
				if !assigned.add(position) {
					return newError(ErrInvalid, key.start, "duplicate destination struct field")
				}
				if err := decoders[position](state, fieldForDecode(destination, fields[position].index), entry); err != nil {
					return err
				}
				hint = position + 1
			}
			child = state.items[entry].next
		}
		if info.required > 0 {
			for position := range fields {
				if fields[position].required && !assigned.has(position) {
					return newError(ErrType, value.start, "missing required field "+keyString(fields[position])+" ("+fields[position].goName+") of "+destination.Type().String())
				}
			}
		}
		return nil
	}
}

// generic converts an item to the natural Go representation used for any.
func (state *decodeState) generic(index int) (any, error) {
	value := &state.items[index]
	switch value.major {
	case 0:
		return value.argument, nil
	case 1:
		if value.argument > math.MaxInt64 {
			return negativeBig(value.argument), nil
		}
		return -1 - int64(value.argument), nil
	case 2:
		return bytes.Clone(state.payload(value)), nil
	case 3:
		return string(state.payload(value)), nil
	case 4:
		values := make([]any, value.argument)
		child := index + 1
		for position := range values {
			decoded, err := state.generic(child)
			if err != nil {
				return nil, err
			}
			values[position] = decoded
			child = state.items[child].next
		}
		return values, nil
	case 5:
		values := make(map[any]any, value.argument)
		child := index + 1
		for range value.argument {
			key := &state.items[child]
			switch {
			case key.major == 2:
				return nil, newError(ErrType, key.start, "byte-string map key cannot be represented in map[any]any")
			case key.major == 1 && key.argument > math.MaxInt64:
				// A *big.Int key would compare by pointer, not by value.
				return nil, newError(ErrType, key.start, "integer map key below math.MinInt64 cannot be represented in map[any]any")
			}
			decodedKey, err := state.generic(child)
			if err != nil {
				return nil, err
			}
			entry := key.next
			decoded, err := state.generic(entry)
			if err != nil {
				return nil, err
			}
			values[decodedKey] = decoded
			child = state.items[entry].next
		}
		return values, nil
	case 6:
		decoded, err := state.generic(index + 1)
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

// tagPackage identifies instantiations of Tag, whose fields the encoder reads
// directly.
var tagPackage = reflect.TypeFor[TaggedValue]().PkgPath()

func isTagType(kind reflect.Type) bool {
	return kind.Kind() == reflect.Struct && kind.PkgPath() == tagPackage && strings.HasPrefix(kind.Name(), "Tag[")
}
