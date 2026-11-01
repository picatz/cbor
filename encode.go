package cbor

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/big"
	"reflect"
	"slices"
	"sync"
	"unicode/utf8"
)

const (
	maxEncodedItem  = 1 << 20
	maxEncodedDepth = 64
)

var (
	errSizeLimit  = encodeError(ErrLimit, "encoding size limit exceeded")
	errDepthLimit = encodeError(ErrLimit, "encoding nesting limit exceeded")
	errArrayLimit = encodeError(ErrLimit, "array size limit exceeded")
	errMapLimit   = encodeError(ErrLimit, "map size limit exceeded")
)

// encodeState carries options and reusable buffers through one encoding.
// States are pooled, so steady-state encoding allocates only its result.
type encodeState struct {
	options       EncoderOptions
	deterministic bool

	// base is the offset in the current buffer where the item being
	// bounded by maxEncodedItem starts.
	base int

	// buffer is reused by Marshal and Encoder.Encode.
	buffer []byte

	// scratch and spans hold each map's encoded entries while they are
	// sorted, one level per nesting depth so nested maps never share.
	scratch [][]byte
	spans   [][]span
}

// span locates one encoded map entry in a scratch buffer.
type span struct{ start, key, end int }

// maxPooledBuffer bounds the buffers a pooled state keeps.
const maxPooledBuffer = 64 << 10

var encodeStatePool = sync.Pool{New: func() any { return new(encodeState) }}

func getEncodeState(options EncoderOptions) *encodeState {
	state := encodeStatePool.Get().(*encodeState)
	state.options = options
	state.deterministic = options.deterministic()
	return state
}

func putEncodeState(state *encodeState) {
	if cap(state.buffer) > maxPooledBuffer {
		state.buffer = nil
	}
	for depth := range state.scratch {
		if cap(state.scratch[depth]) > maxPooledBuffer {
			state.scratch[depth] = nil
		}
		if cap(state.spans[depth]) > DefaultMaxValue/4 {
			state.spans[depth] = nil
		}
	}
	state.base = 0
	encodeStatePool.Put(state)
}

// encoderFunc appends the encoding of value. Callers check depth and size
// limits first; see child.
type encoderFunc func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error)

// child checks the nesting and size limits, then encodes one nested value.
func (state *encodeState) child(encode encoderFunc, output []byte, value reflect.Value, depth int) ([]byte, error) {
	if depth > maxEncodedDepth {
		return nil, errDepthLimit
	}
	if len(output)-state.base >= maxEncodedItem {
		return nil, errSizeLimit
	}
	return encode(state, output, value, depth)
}

// encoderCache maps a reflect.Type to its encoderFunc.
var encoderCache sync.Map

// encoderFor returns the encoder for a type, building it once. Recursive
// types receive an indirect encoder while their own is under construction.
func encoderFor(kind reflect.Type) encoderFunc {
	if cached, ok := encoderCache.Load(kind); ok {
		return cached.(encoderFunc)
	}
	var (
		ready   sync.WaitGroup
		encoder encoderFunc
	)
	ready.Add(1)
	cached, loaded := encoderCache.LoadOrStore(kind, encoderFunc(func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
		ready.Wait()
		return encoder(state, output, value, depth)
	}))
	if loaded {
		return cached.(encoderFunc)
	}
	defer func() {
		if encoder == nil {
			// Construction panicked: release any waiters with an error and
			// drop the placeholder, so the cache is not left poisoned.
			encoderCache.CompareAndDelete(kind, cached)
			encoder = func(*encodeState, []byte, reflect.Value, int) ([]byte, error) {
				return nil, encodeError(ErrUnsupported, "no encoder for "+kind.String())
			}
		}
		ready.Done()
	}()
	built := newEncoder(kind)
	encoder = built
	encoderCache.Store(kind, built)
	return built
}

var marshalerType = reflect.TypeFor[Marshaler]()

func newEncoder(kind reflect.Type) encoderFunc {
	if kind.Kind() == reflect.Interface {
		return encodeInterface
	}
	if kind == rawMessageType {
		return encodeRawMessage
	}
	if kind.Implements(marshalerType) {
		return newMarshalerEncoder(kind)
	}
	switch kind {
	case taggedValueType:
		return encodeTaggedValue
	case simpleValueType:
		return encodeSimpleValue
	case bigIntType:
		return encodeBigInt
	}
	if kind.Kind() == reflect.Struct && kind.Implements(tagEncoderType) {
		return newTagEncoder(kind)
	}
	return newKindEncoder(kind)
}

func newKindEncoder(kind reflect.Type) encoderFunc {
	switch kind.Kind() {
	case reflect.Pointer:
		return newPointerEncoder(kind)
	case reflect.Bool:
		return encodeBool
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return encodeInt
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return encodeUint
	case reflect.Float32, reflect.Float64:
		return encodeFloat
	case reflect.String:
		return encodeString
	case reflect.Slice:
		if kind.Elem().Kind() == reflect.Uint8 {
			return encodeByteSlice
		}
		return newArrayEncoder(kind)
	case reflect.Array:
		if kind.Elem().Kind() == reflect.Uint8 {
			return encodeByteArray
		}
		return newArrayEncoder(kind)
	case reflect.Map:
		return newMapEncoder(kind)
	case reflect.Struct:
		return newStructEncoder(kind)
	}
	return func(*encodeState, []byte, reflect.Value, int) ([]byte, error) {
		return nil, encodeError(ErrUnsupported, "unsupported type "+kind.String())
	}
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
	}
	return binary.BigEndian.AppendUint64(append(output, marker|27), argument)
}

func appendInt(output []byte, number int64) []byte {
	if number < 0 {
		return appendHeader(output, 1, uint64(-(number + 1)))
	}
	return appendHeader(output, 0, uint64(number))
}

func appendBool(output []byte, value bool) []byte {
	if value {
		return append(output, 0xf5)
	}
	return append(output, 0xf4)
}

func (state *encodeState) appendText(output []byte, text string) ([]byte, error) {
	if !utf8.ValidString(text) {
		return nil, encodeError(ErrInvalid, "invalid UTF-8")
	}
	if len(text) > maxEncodedItem-(len(output)-state.base)-9 {
		return nil, errSizeLimit
	}
	return append(appendHeader(output, 3, uint64(len(text))), text...), nil
}

func (state *encodeState) appendBytes(output, data []byte) ([]byte, error) {
	if len(data) > maxEncodedItem-(len(output)-state.base)-9 {
		return nil, errSizeLimit
	}
	return append(appendHeader(output, 2, uint64(len(data))), data...), nil
}

func (state *encodeState) appendFloat(output []byte, number float64) []byte {
	if state.options.Floats == FloatShortest {
		minor, bits := shortestFloat(number)
		switch minor {
		case 25:
			return append(output, 0xf9, byte(bits>>8), byte(bits))
		case 26:
			return binary.BigEndian.AppendUint32(append(output, 0xfa), uint32(bits))
		}
	}
	return binary.BigEndian.AppendUint64(append(output, 0xfb), math.Float64bits(number))
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

// appendRaw validates an embedded item, which must fit in the remaining
// nesting budget and, for deterministic output, be deterministic itself.
func (state *encodeState) appendRaw(output, encoded []byte, depth int) ([]byte, error) {
	if len(encoded) > maxEncodedItem-(len(output)-state.base) {
		return nil, errSizeLimit
	}
	if err := validateRaw(encoded, depth, state.deterministic); err != nil {
		return nil, err
	}
	return append(output, encoded...), nil
}

// appendItem appends one complete item to output. Limits are checked before
// each nested value, so the last headers and scalars are checked here.
func (state *encodeState) appendItem(output []byte, value any) ([]byte, error) {
	state.base = len(output)
	encoded, err := state.appendAny(output, value, 0)
	if err != nil {
		return nil, err
	}
	if len(encoded)-state.base > maxEncodedItem {
		return nil, errSizeLimit
	}
	return encoded, nil
}

// appendAny encodes a dynamic value. The types that decoding into any
// produces, and other common ones, are encoded without reflection.
func (state *encodeState) appendAny(output []byte, value any, depth int) ([]byte, error) {
	switch value := value.(type) {
	case nil:
		return append(output, 0xf6), nil
	case bool:
		return appendBool(output, value), nil
	case string:
		return state.appendText(output, value)
	case uint64:
		return appendHeader(output, 0, value), nil
	case int64:
		return appendInt(output, value), nil
	case int:
		return appendInt(output, int64(value)), nil
	case uint:
		return appendHeader(output, 0, uint64(value)), nil
	case uint32:
		return appendHeader(output, 0, uint64(value)), nil
	case int32:
		return appendInt(output, int64(value)), nil
	case float64:
		return state.appendFloat(output, value), nil
	case []byte:
		if value == nil {
			return append(output, 0xf6), nil
		}
		return state.appendBytes(output, value)
	case []any:
		if value == nil {
			return append(output, 0xf6), nil
		}
		if len(value) > DefaultMaxValue {
			return nil, errArrayLimit
		}
		if len(value) > 0 && depth+1 > maxEncodedDepth {
			return nil, errDepthLimit
		}
		output = appendHeader(output, 4, uint64(len(value)))
		for _, element := range value {
			if len(output)-state.base >= maxEncodedItem {
				return nil, errSizeLimit
			}
			var err error
			if output, err = state.appendAny(output, element, depth+1); err != nil {
				return nil, err
			}
		}
		return output, nil
	case map[string]any:
		if value == nil {
			return append(output, 0xf6), nil
		}
		return state.appendStringMap(output, value, depth)
	case map[any]any:
		if value == nil {
			return append(output, 0xf6), nil
		}
		return state.appendGenericMap(output, value, depth)
	case TaggedValue:
		if depth+1 > maxEncodedDepth {
			return nil, errDepthLimit
		}
		return state.appendAny(appendHeader(output, 6, value.Number), value.Value, depth+1)
	case RawMessage:
		if value == nil {
			return append(output, 0xf6), nil
		}
		return state.appendRaw(output, value, depth)
	case *big.Int:
		if value == nil {
			return append(output, 0xf6), nil
		}
		return appendBigInt(output, value), nil
	}
	reflected := reflect.ValueOf(value)
	return encoderFor(reflected.Type())(state, output, reflected, depth)
}

func encodeInterface(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
	if value.IsNil() {
		return append(output, 0xf6), nil
	}
	if value.CanInterface() {
		// Interfaces add no CBOR nesting, so they do not consume depth.
		return state.appendAny(output, value.Interface(), depth)
	}
	element := value.Elem()
	return encoderFor(element.Type())(state, output, element, depth)
}

func newPointerEncoder(kind reflect.Type) encoderFunc {
	element := encoderFor(kind.Elem())
	return func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
		if value.IsNil() {
			return append(output, 0xf6), nil
		}
		// Pointers consume depth so that pointer cycles terminate.
		return state.child(element, output, value.Elem(), depth+1)
	}
}

func encodeRawMessage(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
	if value.IsNil() {
		return append(output, 0xf6), nil
	}
	return state.appendRaw(output, value.Bytes(), depth)
}

func newMarshalerEncoder(kind reflect.Type) encoderFunc {
	fallback := newKindEncoder(kind)
	pointer := kind.Kind() == reflect.Pointer
	return func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
		if pointer && value.IsNil() {
			return append(output, 0xf6), nil
		}
		if !value.CanInterface() {
			return fallback(state, output, value, depth)
		}
		encoded, err := value.Interface().(Marshaler).MarshalCBOR()
		if err != nil {
			return nil, err
		}
		return state.appendRaw(output, encoded, depth)
	}
}

func encodeTaggedValue(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
	output = appendHeader(output, 6, value.Field(0).Uint())
	return state.child(encodeInterface, output, value.Field(1), depth+1)
}

func encodeSimpleValue(_ *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	return appendSimple(output, SimpleValue(value.Int()))
}

func encodeBigInt(_ *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	if value.CanAddr() {
		return appendBigInt(output, value.Addr().Interface().(*big.Int)), nil
	}
	number := value.Interface().(big.Int)
	return appendBigInt(output, &number), nil
}

func newTagEncoder(kind reflect.Type) encoderFunc {
	number, content, ok := tagFields(kind)
	if !ok {
		return func(*encodeState, []byte, reflect.Value, int) ([]byte, error) {
			return nil, encodeError(ErrUnsupported, "ambiguous or unexported embedded Tag in "+kind.String())
		}
	}
	contentField := kind.FieldByIndex(content)
	encode := encoderFor(contentField.Type)
	return func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
		tagNumber, ok := fieldByIndex(value, number)
		if !ok {
			return append(output, 0xf6), nil // a nil embedded *Tag
		}
		field, _ := fieldByIndex(value, content)
		return state.child(encode, appendHeader(output, 6, tagNumber.Uint()), field, depth+1)
	}
}

func encodeBool(_ *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	return appendBool(output, value.Bool()), nil
}

func encodeInt(_ *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	return appendInt(output, value.Int()), nil
}

func encodeUint(_ *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	return appendHeader(output, 0, value.Uint()), nil
}

func encodeFloat(state *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	return state.appendFloat(output, value.Float()), nil
}

func encodeString(state *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	return state.appendText(output, value.String())
}

// encodeByteSlice encodes []byte, and slices of other byte types, as a byte
// string. A nil slice encodes as null.
func encodeByteSlice(state *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	if value.IsNil() {
		return append(output, 0xf6), nil
	}
	return state.appendBytes(output, value.Bytes())
}

// encodeByteArray encodes byte arrays such as [16]byte as byte strings.
func encodeByteArray(state *encodeState, output []byte, value reflect.Value, _ int) ([]byte, error) {
	if value.CanAddr() {
		return state.appendBytes(output, value.Bytes())
	}
	length := value.Len()
	if length > maxEncodedItem-(len(output)-state.base)-9 {
		return nil, errSizeLimit
	}
	output = appendHeader(output, 2, uint64(length))
	for index := range length {
		output = append(output, byte(value.Index(index).Uint()))
	}
	return output, nil
}

func newArrayEncoder(kind reflect.Type) encoderFunc {
	element := encoderFor(kind.Elem())
	slice := kind.Kind() == reflect.Slice
	return func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
		if slice && value.IsNil() {
			return append(output, 0xf6), nil
		}
		length := value.Len()
		if length > DefaultMaxValue {
			return nil, errArrayLimit
		}
		output = appendHeader(output, 4, uint64(length))
		for index := range length {
			var err error
			if output, err = state.child(element, output, value.Index(index), depth+1); err != nil {
				return nil, err
			}
		}
		return output, nil
	}
}

func newStructEncoder(kind reflect.Type) encoderFunc {
	info, err := structFor(kind)
	if err != nil {
		return func(*encodeState, []byte, reflect.Value, int) ([]byte, error) { return nil, err }
	}
	fields := info.fields
	encoders := make([]encoderFunc, len(fields))
	optional := false
	for position := range fields {
		encoders[position] = encoderFor(fields[position].typ)
		optional = optional || fields[position].omitEmpty || fields[position].omitZero || len(fields[position].index) > 1
	}
	if info.toArray {
		return func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
			output = appendHeader(output, 4, uint64(len(fields)))
			for position := range fields {
				field, _ := fieldByIndex(value, fields[position].index)
				var err error
				if output, err = state.child(encoders[position], output, field, depth+1); err != nil {
					return nil, err
				}
			}
			return output, nil
		}
	}
	return func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
		order := info.order[state.options.Sort]
		if !optional {
			output = appendHeader(output, 5, uint64(len(fields)))
			for _, position := range order {
				output = append(output, fields[position].encodedKey...)
				var err error
				if output, err = state.child(encoders[position], output, value.Field(fields[position].index[0]), depth+1); err != nil {
					return nil, err
				}
			}
			return output, nil
		}
		// Select the present fields in one pass, on the stack for typical
		// structs, so each field is located and tested once.
		var buffer [24]presentField
		present := buffer[:0]
		for _, position := range order {
			if field, ok := fieldByIndex(value, fields[position].index); ok && !fields[position].omit(field) {
				present = append(present, presentField{position: position, value: field})
			}
		}
		output = appendHeader(output, 5, uint64(len(present)))
		for _, entry := range present {
			output = append(output, fields[entry.position].encodedKey...)
			var err error
			if output, err = state.child(encoders[entry.position], output, entry.value, depth+1); err != nil {
				return nil, err
			}
		}
		return output, nil
	}
}

// presentField is a struct field selected for encoding.
type presentField struct {
	position int
	value    reflect.Value
}

// uniqueKeys reports whether distinct Go keys of this type always have
// distinct CBOR encodings, so no data-model duplicate check is needed.
func uniqueKeys(kind reflect.Type) bool {
	if kind.NumMethod() > 0 {
		return false
	}
	switch kind.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return true
	}
	return false
}

func newMapEncoder(kind reflect.Type) encoderFunc {
	keyType, elementType := kind.Key(), kind.Elem()
	encodeKey, encodeElement := encoderFor(keyType), encoderFor(elementType)
	checkKeys := !uniqueKeys(keyType)
	return func(state *encodeState, output []byte, value reflect.Value, depth int) ([]byte, error) {
		if value.IsNil() {
			return append(output, 0xf6), nil
		}
		length := value.Len()
		if length > DefaultMaxValue {
			return nil, errMapLimit
		}
		if length == 0 {
			return append(output, 0xa0), nil
		}
		entries := state.beginMap(depth)
		key, element := reflect.New(keyType).Elem(), reflect.New(elementType).Elem()
		var iterator reflect.MapIter
		iterator.Reset(value)
		for iterator.Next() {
			key.SetIterKey(&iterator)
			element.SetIterValue(&iterator)
			start := len(entries.buffer)
			buffer, err := state.child(encodeKey, entries.buffer, key, depth+1)
			if err != nil {
				return nil, err
			}
			keyEnd := len(buffer)
			if buffer, err = state.child(encodeElement, buffer, element, depth+1); err != nil {
				return nil, err
			}
			entries.add(buffer, start, keyEnd)
		}
		return state.endMap(output, depth, entries, checkKeys)
	}
}

// appendStringMap encodes map[string]any without reflection.
func (state *encodeState) appendStringMap(output []byte, value map[string]any, depth int) ([]byte, error) {
	if len(value) > DefaultMaxValue {
		return nil, errMapLimit
	}
	if len(value) > 0 && depth+1 > maxEncodedDepth {
		return nil, errDepthLimit
	}
	entries := state.beginMap(depth)
	for key, element := range value {
		start := len(entries.buffer)
		buffer, err := state.appendText(entries.buffer, key)
		if err != nil {
			return nil, err
		}
		keyEnd := len(buffer)
		if len(buffer)-state.base >= maxEncodedItem {
			return nil, errSizeLimit
		}
		if buffer, err = state.appendAny(buffer, element, depth+1); err != nil {
			return nil, err
		}
		entries.add(buffer, start, keyEnd)
	}
	return state.endMap(output, depth, entries, false)
}

// appendGenericMap encodes map[any]any without reflection.
func (state *encodeState) appendGenericMap(output []byte, value map[any]any, depth int) ([]byte, error) {
	if len(value) > DefaultMaxValue {
		return nil, errMapLimit
	}
	if len(value) > 0 && depth+1 > maxEncodedDepth {
		return nil, errDepthLimit
	}
	entries := state.beginMap(depth)
	for key, element := range value {
		start := len(entries.buffer)
		if len(entries.buffer)-state.base >= maxEncodedItem {
			return nil, errSizeLimit
		}
		buffer, err := state.appendAny(entries.buffer, key, depth+1)
		if err != nil {
			return nil, err
		}
		keyEnd := len(buffer)
		if len(buffer)-state.base >= maxEncodedItem {
			return nil, errSizeLimit
		}
		if buffer, err = state.appendAny(buffer, element, depth+1); err != nil {
			return nil, err
		}
		entries.add(buffer, start, keyEnd)
	}
	return state.endMap(output, depth, entries, true)
}

// mapEntries accumulates one map's encoded entries in the scratch buffer for
// its depth.
type mapEntries struct {
	buffer []byte
	spans  []span
	base   int // the state's base before the map started
}

func (entries *mapEntries) add(buffer []byte, start, keyEnd int) {
	entries.buffer = buffer
	entries.spans = append(entries.spans, span{start: start, key: keyEnd, end: len(buffer)})
}

// beginMap borrows the scratch buffers for depth. Scratch offsets start at
// zero, so size checks inside the map are relative to the scratch buffer
// until endMap restores the base. An encoding error abandons the state's
// base, which is reset when the state returns to the pool.
func (state *encodeState) beginMap(depth int) mapEntries {
	for len(state.scratch) <= depth {
		state.scratch = append(state.scratch, nil)
		state.spans = append(state.spans, nil)
	}
	entries := mapEntries{buffer: state.scratch[depth][:0], spans: state.spans[depth][:0], base: state.base}
	state.base = 0
	return entries
}

// endMap sorts the entries, appends the map to output and returns the
// scratch buffers for reuse.
func (state *encodeState) endMap(output []byte, depth int, entries mapEntries, checkKeys bool) ([]byte, error) {
	state.base = entries.base
	buffer, spans := entries.buffer, entries.spans
	state.scratch[depth], state.spans[depth] = buffer[:0], spans[:0]
	if checkKeys {
		if err := checkEncodedKeys(buffer, spans); err != nil {
			return nil, err
		}
	}
	mode := state.options.Sort
	slices.SortFunc(spans, func(left, right span) int {
		return compareKeys(mode, buffer[left.start:left.key], buffer[right.start:right.key])
	})
	if len(buffer) > maxEncodedItem-(len(output)-state.base)-9 {
		return nil, errSizeLimit
	}
	output = appendHeader(output, 5, uint64(len(spans)))
	for _, entry := range spans {
		output = append(output, buffer[entry.start:entry.end]...)
	}
	return output, nil
}

// checkEncodedKeys rejects key types CBOR maps cannot hold and keys that are
// equal in the CBOR data model, such as 1.0 as float32 and float64.
func checkEncodedKeys(buffer []byte, spans []span) error {
	var seen map[keyID]struct{}
	for _, entry := range spans {
		parser := parser{options: &rawOptions}
		parser.setInput(buffer[entry.start:entry.key])
		var key item
		if err := parser.skip(&key, 0); err != nil {
			return err
		}
		if err := checkKeyType(&key); err != nil {
			return err
		}
		if seen == nil {
			seen = make(map[keyID]struct{}, len(spans))
		}
		id := identify(parser.data, &key)
		if _, ok := seen[id]; ok {
			return encodeError(ErrInvalid, "duplicate encoded map key")
		}
		seen[id] = struct{}{}
	}
	return nil
}

// compareKeys orders encoded map keys for a SortMode.
func compareKeys(mode SortMode, left, right []byte) int {
	if mode == SortLengthFirst && len(left) != len(right) {
		return len(left) - len(right)
	}
	return bytes.Compare(left, right)
}
