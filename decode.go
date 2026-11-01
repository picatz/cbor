package cbor

import (
	"bytes"
	"math"
	"math/big"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
)

// decodeState converts items into Go values as the parser reads them.
// Decoded values never alias the parser's data.
//
// Every decoderFunc writes into a zero value: the destination is a fresh
// replacement, and containers reset their scratch elements between entries.
// Nothing a decoderFunc does is observable until the whole item has been
// validated: calls into application code, and the map insertions that would
// copy their receivers, are queued in pending and run by flush afterwards.
type decodeState struct {
	parser
	rejectUnknown bool // reject keys that match no struct field exactly
	deterministic bool // require preferred serialization of bignums
	pending       []pendingOp
	scratch       DecoderOptions // options, when not the shared defaults
}

// pendingOp is a deferred UnmarshalCBOR call on receiver, or when receiver
// is nil, a deferred map insertion whose key or element awaits such a call.
type pendingOp struct {
	receiver   Unmarshaler
	start, end int // the receiver's item, or the key's offset for errors
	target     reflect.Value
	key        reflect.Value
	element    reflect.Value
}

var statePool = sync.Pool{New: func() any { return new(decodeState) }}

// getState returns a pooled state configured with options, which the state
// copies unless it is the shared defaults, so the caller's value need not
// outlive the call or leave its stack.
func getState(options *DecoderOptions) *decodeState {
	state := statePool.Get().(*decodeState)
	if options == &defaultOptions {
		state.options = &defaultOptions
	} else {
		state.scratch = *options
		state.options = &state.scratch
	}
	state.rejectUnknown = options.RejectUnknownFields
	state.deterministic = options.RequireDeterministic
	return state
}

// maxPooledPending bounds the queue kept for reuse, so one large item does
// not pin memory for the life of the process.
const maxPooledPending = 4096

func putState(state *decodeState) {
	state.reader, state.data, state.pos, state.limit = nil, nil, 0, 0
	if cap(state.pending) > maxPooledPending {
		state.pending = nil
	}
	clear(state.pending)
	state.pending = state.pending[:0]
	statePool.Put(state)
}

// enqueue queues an UnmarshalCBOR call on the validated item value.
func (state *decodeState) enqueue(receiver Unmarshaler, value *item) {
	state.pending = append(state.pending, pendingOp{receiver: receiver, start: value.start, end: value.end})
}

// flush runs the queued operations in order, once the whole item is valid.
func (state *decodeState) flush() error {
	for position := range state.pending {
		op := &state.pending[position]
		if op.receiver != nil {
			if err := op.receiver.UnmarshalCBOR(state.data[op.start:op.end:op.end]); err != nil {
				return err
			}
			continue
		}
		if err := insert(op.target, op.key, op.element, op.start); err != nil {
			return err
		}
	}
	return nil
}

// insert adds one entry to a decoded map. Keys equal in the CBOR data model
// decode to equal Go keys, and so can distinct keys, such as through an
// Unmarshaler; either is a duplicate.
func insert(target, key, element reflect.Value, offset int) error {
	before := target.Len()
	target.SetMapIndex(key, element)
	if target.Len() == before {
		return newError(ErrInvalid, offset, "duplicate map key")
	}
	return nil
}

// valueKeys reports whether a map with keys of kind detects every pair of
// keys equal in the CBOR data model through insert alone, because such keys
// decode, without application code, to Go values that compare equal. Keys
// holding pointers compare by address, so those maps also keep a keySet.
func valueKeys(kind reflect.Type) bool {
	if deferred(kind) {
		return false
	}
	switch kind.Kind() {
	case reflect.Bool, reflect.String, reflect.Interface,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Array:
		return valueKeys(kind.Elem())
	}
	return false
}

// decoderFunc decodes the item whose header value was read by parser.item
// at nesting depth into destination, which is addressable and holds its
// type's zero value. It consumes the item's remaining bytes.
type decoderFunc func(state *decodeState, destination reflect.Value, value item, depth int) error

// typeDecoder is a cache entry: a type and its decoder. Entries are
// immutable, so the one looked up last can be published without a lock.
type typeDecoder struct {
	kind        reflect.Type
	decode      decoderFunc
	placeholder bool // decode waits for the type's decoder to be built
}

// decoderCache maps a reflect.Type to its *typeDecoder, and lastDecoder is
// the entry looked up last, which a caller decoding one type repeatedly
// finds without hashing.
var (
	decoderCache sync.Map
	lastDecoder  atomic.Pointer[typeDecoder]
)

// decoderFor returns the decoder for a type, building it once.
func decoderFor(kind reflect.Type) decoderFunc {
	if last := lastDecoder.Load(); last != nil && last.kind == kind {
		return last.decode
	}
	entry := decoderEntry(kind)
	if !entry.placeholder {
		lastDecoder.Store(entry)
	}
	return entry.decode
}

// decoderEntry returns the cache entry for a type, building its decoder
// once. Recursive types receive a placeholder entry while their own is under
// construction.
func decoderEntry(kind reflect.Type) *typeDecoder {
	if cached, ok := decoderCache.Load(kind); ok {
		return cached.(*typeDecoder)
	}
	var (
		ready   sync.WaitGroup
		decoder decoderFunc
	)
	ready.Add(1)
	placeholder := &typeDecoder{kind: kind, placeholder: true, decode: func(state *decodeState, destination reflect.Value, value item, depth int) error {
		ready.Wait()
		return decoder(state, destination, value, depth)
	}}
	if cached, loaded := decoderCache.LoadOrStore(kind, placeholder); loaded {
		return cached.(*typeDecoder)
	}
	defer func() {
		if decoder == nil {
			// Construction panicked: release any waiters with an error and
			// drop the placeholder, so the cache is not left poisoned.
			decoderCache.CompareAndDelete(kind, placeholder)
			decoder = func(*decodeState, reflect.Value, item, int) error {
				return encodeError(ErrUnsupported, "no decoder for "+kind.String())
			}
		}
		ready.Done()
	}()
	built := &typeDecoder{kind: kind, decode: newDecoder(kind)}
	decoder = built.decode
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

// deferred reports whether decoding into kind can queue an UnmarshalCBOR
// call, which writes to its receiver only when flush runs. A map whose keys
// or elements can must not copy them into the map before then.
func deferred(kind reflect.Type) bool {
	return deferredWithin(kind, map[reflect.Type]bool{})
}

func deferredWithin(kind reflect.Type, visited map[reflect.Type]bool) bool {
	if kind == rawMessageType {
		return false // decoded directly, without calling UnmarshalCBOR
	}
	if kind.Implements(unmarshalerType) || kind.Kind() != reflect.Pointer && reflect.PointerTo(kind).Implements(unmarshalerType) {
		return true
	}
	if visited[kind] {
		return false
	}
	visited[kind] = true
	switch kind.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Array:
		return deferredWithin(kind.Elem(), visited)
	case reflect.Map:
		return deferredWithin(kind.Key(), visited) || deferredWithin(kind.Elem(), visited)
	case reflect.Struct:
		for position := range kind.NumField() {
			if deferredWithin(kind.Field(position).Type, visited) {
				return true
			}
		}
	}
	return false
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

func decodeUnsupported(state *decodeState, destination reflect.Value, value item, depth int) error {
	return state.typeError(&value, destination.Type())
}

func decodeUnmarshalerPointer(state *decodeState, destination reflect.Value, value item, depth int) error {
	if err := state.skipRest(&value, depth); err != nil {
		return err
	}
	destination.Set(reflect.New(destination.Type().Elem()))
	state.enqueue(destination.Interface().(Unmarshaler), &value)
	return nil
}

func decodeUnmarshaler(state *decodeState, destination reflect.Value, value item, depth int) error {
	if err := state.skipRest(&value, depth); err != nil {
		return err
	}
	state.enqueue(destination.Addr().Interface().(Unmarshaler), &value)
	return nil
}

func decodeRawMessage(state *decodeState, destination reflect.Value, value item, depth int) error {
	if err := state.skipRest(&value, depth); err != nil {
		return err
	}
	destination.SetBytes(bytes.Clone(state.data[value.start:value.end]))
	return nil
}

func newPointerDecoder(kind reflect.Type) decoderFunc {
	element := kind.Elem()
	decode := decoderFor(element)
	return func(state *decodeState, destination reflect.Value, value item, depth int) error {
		if value.isNull() {
			return nil
		}
		allocated := reflect.New(element)
		if err := decode(state, allocated.Elem(), value, depth); err != nil {
			return err
		}
		destination.Set(allocated)
		return nil
	}
}

func decodeAny(state *decodeState, destination reflect.Value, value item, depth int) error {
	if value.isNull() {
		return nil
	}
	decoded, err := state.generic(&value, depth)
	if err != nil {
		return err
	}
	*destination.Addr().Interface().(*any) = decoded
	return nil
}

func decodeInterface(state *decodeState, destination reflect.Value, value item, depth int) error {
	if value.isNull() {
		return nil
	}
	decoded, err := state.generic(&value, depth)
	if err != nil {
		return err
	}
	reflected := reflect.ValueOf(decoded)
	if !reflected.Type().AssignableTo(destination.Type()) {
		return state.typeError(&value, destination.Type())
	}
	destination.Set(reflected)
	return nil
}

func decodeTaggedValue(state *decodeState, destination reflect.Value, value item, depth int) error {
	if value.major != 6 {
		return state.typeError(&value, destination.Type())
	}
	decoded, err := state.generic(&value, depth)
	if err != nil {
		return err
	}
	*destination.Addr().Interface().(*TaggedValue) = decoded.(TaggedValue)
	return nil
}

func decodeSimpleValue(state *decodeState, destination reflect.Value, value item, depth int) error {
	if value.major != 7 || value.isFloat() || value.isNull() {
		return state.typeError(&value, destination.Type())
	}
	destination.SetInt(int64(value.argument))
	return nil
}

func newTagDecoder(kind reflect.Type) decoderFunc {
	number, content, ok := tagFields(kind)
	if !ok {
		return func(*decodeState, reflect.Value, item, int) error {
			return encodeError(ErrUnsupported, "ambiguous or unexported embedded Tag in "+kind.String())
		}
	}
	contentField := kind.FieldByIndex(content)
	decode := decoderFor(contentField.Type)
	return func(state *decodeState, destination reflect.Value, value item, depth int) error {
		if value.major != 6 {
			return state.typeError(&value, destination.Type())
		}
		var child item
		if err := state.item(&child, depth+1); err != nil {
			return err
		}
		fieldForDecode(destination, number).SetUint(value.argument)
		return decode(state, fieldForDecode(destination, content), child, depth+1)
	}
}

func decodeBool(state *decodeState, destination reflect.Value, value item, depth int) error {
	if value.major == 7 && (value.minor == 20 || value.minor == 21) {
		destination.SetBool(value.minor == 21)
		return nil
	}
	return state.typeError(&value, destination.Type())
}

func decodeString(state *decodeState, destination reflect.Value, value item, depth int) error {
	if value.major != 3 {
		return state.typeError(&value, destination.Type())
	}
	destination.SetString(string(state.payload(&value)))
	return nil
}

func decodeInt(state *decodeState, destination reflect.Value, value item, depth int) error {
	var decoded int64
	switch {
	case value.major == 0 && value.argument <= math.MaxInt64:
		decoded = int64(value.argument)
	case value.major == 1 && value.argument <= math.MaxInt64:
		decoded = -1 - int64(value.argument)
	case value.major == 0 || value.major == 1:
		return state.overflow(&value, destination.Type())
	default:
		return state.typeError(&value, destination.Type())
	}
	if destination.OverflowInt(decoded) {
		return state.overflow(&value, destination.Type())
	}
	destination.SetInt(decoded)
	return nil
}

func decodeUint(state *decodeState, destination reflect.Value, value item, depth int) error {
	switch {
	case value.major == 1:
		return state.overflow(&value, destination.Type())
	case value.major != 0:
		return state.typeError(&value, destination.Type())
	case destination.OverflowUint(value.argument):
		return state.overflow(&value, destination.Type())
	}
	destination.SetUint(value.argument)
	return nil
}

func decodeFloat(state *decodeState, destination reflect.Value, value item, depth int) error {
	if value.isFloat() {
		decoded := value.float()
		if destination.OverflowFloat(decoded) {
			return state.overflow(&value, destination.Type())
		}
		destination.SetFloat(decoded)
		return nil
	}
	decoded, ok := exactInteger(&value)
	if !ok || destination.Kind() == reflect.Float32 && float64(float32(decoded)) != decoded {
		return state.typeError(&value, destination.Type())
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
	return func(state *decodeState, destination reflect.Value, value item, depth int) error {
		switch {
		case value.isNull():
			return nil
		case byteString && value.major == 2:
			destination.SetBytes(append(make([]byte, 0, value.argument), state.payload(&value)...))
			return nil
		case value.major != 4:
			return state.typeError(&value, destination.Type())
		}
		count := int(value.argument)
		slice := reflect.MakeSlice(kind, count, count)
		var child item
		for position := range count {
			if err := state.item(&child, depth+1); err != nil {
				return err
			}
			if err := element(state, slice.Index(position), child, depth+1); err != nil {
				return err
			}
		}
		destination.Set(slice)
		return nil
	}
}

func newArrayDecoder(kind reflect.Type) decoderFunc {
	byteString := kind.Elem().Kind() == reflect.Uint8
	element := decoderFor(kind.Elem())
	length := kind.Len()
	return func(state *decodeState, destination reflect.Value, value item, depth int) error {
		if byteString && value.major == 2 && value.argument == uint64(length) {
			copy(destination.Bytes(), state.payload(&value))
			return nil
		}
		if value.major != 4 || value.argument != uint64(length) {
			return state.typeError(&value, destination.Type())
		}
		var child item
		for position := range length {
			if err := state.item(&child, depth+1); err != nil {
				return err
			}
			if err := element(state, destination.Index(position), child, depth+1); err != nil {
				return err
			}
		}
		return nil
	}
}

func newMapDecoder(kind reflect.Type) decoderFunc {
	keyType, elementType := kind.Key(), kind.Elem()
	decodeKey, decodeElement := decoderFor(keyType), decoderFor(elementType)
	interfaceKey := keyType.Kind() == reflect.Interface
	// Entries whose key or element awaits an UnmarshalCBOR call are decoded
	// into fresh values and inserted by flush, after that call has run.
	deferInsert := deferred(keyType) || deferred(elementType)
	// Deferred insertions cannot detect duplicates until flush, after
	// application code has run, so those maps track keys during the pass.
	trackKeys := deferInsert || !valueKeys(keyType)
	return func(state *decodeState, destination reflect.Value, value item, depth int) error {
		if value.isNull() {
			return nil
		}
		if value.major != 5 {
			return state.typeError(&value, destination.Type())
		}
		count := int(value.argument)
		result := reflect.MakeMapWithSize(kind, count)
		var key, element reflect.Value
		if !deferInsert {
			key, element = reflect.New(keyType).Elem(), reflect.New(elementType).Elem()
		}
		var keys keySet
		var keyItem, child item
		previous := -1
		for range count {
			if err := state.readKey(&keyItem, previous, depth+1); err != nil {
				return err
			}
			if trackKeys {
				if err := keys.add(state.data, &keyItem); err != nil {
					return err
				}
			}
			previous = keyItem.start
			if interfaceKey && keyItem.major == 1 && keyItem.argument > math.MaxInt64 {
				// A *big.Int key would compare by pointer, not by value.
				return newError(ErrType, keyItem.start, "integer map key below math.MinInt64 cannot be an interface map key")
			}
			if deferInsert {
				key, element = reflect.New(keyType).Elem(), reflect.New(elementType).Elem()
			} else {
				key.SetZero()
				element.SetZero()
			}
			if err := decodeKey(state, key, keyItem, depth+1); err != nil {
				return err
			}
			if interfaceKey && !key.IsNil() && !key.Elem().Type().Comparable() {
				return newError(ErrType, keyItem.start, "map key cannot be represented in Go")
			}
			if err := state.item(&child, depth+1); err != nil {
				return err
			}
			if err := decodeElement(state, element, child, depth+1); err != nil {
				return err
			}
			if deferInsert {
				state.pending = append(state.pending, pendingOp{start: keyItem.start, target: result, key: key, element: element})
			} else if err := insert(result, key, element, keyItem.start); err != nil {
				return err
			}
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
	return set.addLarge(position)
}

func (set *fieldSet) addLarge(position int) bool {
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
		return func(*decodeState, reflect.Value, item, int) error { return err }
	}
	fields := info.fields
	decoders := make([]decoderFunc, len(fields))
	// direct holds the index of a field declared on the struct itself, or
	// -1 for one promoted from an embedded struct.
	direct := make([]int, len(fields))
	for position := range fields {
		decoders[position] = decoderFor(fields[position].typ)
		direct[position] = -1
		if len(fields[position].index) == 1 {
			direct[position] = fields[position].index[0]
		}
	}
	if info.toArray {
		return func(state *decodeState, destination reflect.Value, value item, depth int) error {
			if value.major != 4 {
				return state.typeError(&value, destination.Type())
			}
			if value.argument != uint64(len(fields)) {
				return newError(ErrType, value.start, "array length does not match the fields of "+destination.Type().String())
			}
			var child item
			for position := range fields {
				if err := state.item(&child, depth+1); err != nil {
					return err
				}
				if err := decoders[position](state, fieldForDecode(destination, fields[position].index), child, depth+1); err != nil {
					return err
				}
			}
			return nil
		}
	}
	return func(state *decodeState, destination reflect.Value, value item, depth int) error {
		if value.major != 5 {
			return state.typeError(&value, destination.Type())
		}
		// Keys that name a field are deduplicated through assigned, since
		// keys equal in the data model name the same field. Keys that name
		// none go through keys, as in any map.
		var assigned fieldSet
		var keys keySet
		var key, child item
		hint, previous := 0, -1
		for range value.argument {
			if err := state.readKey(&key, previous, depth+1); err != nil {
				return err
			}
			previous = key.start
			position := info.lookup(state.data, &key, hint)
			if position < 0 {
				if state.rejectUnknown {
					return newError(ErrType, key.start, "unknown struct field")
				}
				if position = info.fold(state.data, &key); position == -2 {
					return newError(ErrType, key.start, "ambiguous struct field")
				}
			}
			if position < 0 {
				if err := keys.add(state.data, &key); err != nil {
					return err
				}
				if err := state.skip(&child, depth+1); err != nil {
					return err
				}
				continue
			}
			if !assigned.add(position) {
				return newError(ErrInvalid, key.start, "duplicate destination struct field")
			}
			if err := state.item(&child, depth+1); err != nil {
				return err
			}
			var field reflect.Value
			if direct[position] >= 0 {
				field = destination.Field(direct[position])
			} else {
				field = fieldForDecode(destination, fields[position].index)
			}
			if err := decoders[position](state, field, child, depth+1); err != nil {
				return err
			}
			hint = position + 1
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
func (state *decodeState) generic(value *item, depth int) (any, error) {
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
		var child item
		for position := range values {
			if err := state.item(&child, depth+1); err != nil {
				return nil, err
			}
			decoded, err := state.generic(&child, depth+1)
			if err != nil {
				return nil, err
			}
			values[position] = decoded
		}
		return values, nil
	case 5:
		values := make(map[any]any, value.argument)
		var key, child item
		previous := -1
		for range value.argument {
			if err := state.readKey(&key, previous, depth+1); err != nil {
				return nil, err
			}
			previous = key.start
			switch {
			case key.major == 2:
				return nil, newError(ErrType, key.start, "byte-string map key cannot be represented in map[any]any")
			case key.major == 1 && key.argument > math.MaxInt64:
				// A *big.Int key would compare by pointer, not by value.
				return nil, newError(ErrType, key.start, "integer map key below math.MinInt64 cannot be represented in map[any]any")
			}
			decodedKey, err := state.generic(&key, depth+1)
			if err != nil {
				return nil, err
			}
			if err := state.item(&child, depth+1); err != nil {
				return nil, err
			}
			decoded, err := state.generic(&child, depth+1)
			if err != nil {
				return nil, err
			}
			// Keys equal in the data model decode to equal Go values, so the
			// map detects duplicates, as insert does for typed maps.
			before := len(values)
			values[decodedKey] = decoded
			if len(values) == before {
				return nil, newError(ErrInvalid, key.start, "duplicate map key")
			}
		}
		return values, nil
	case 6:
		var child item
		if err := state.item(&child, depth+1); err != nil {
			return nil, err
		}
		decoded, err := state.generic(&child, depth+1)
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
