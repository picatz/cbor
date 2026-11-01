package cbor

import (
	"bytes"
	"io"
	"math"
	"sync"
	"unicode/utf8"
)

// item is one well-formed, valid data item recorded by the parser. Scalars
// stay in their encoded form so parsing allocates nothing per value.
//
// Items live in a flat arena in pre-order: a container's children follow it
// directly, and next is the arena index just past its subtree. Siblings are
// therefore reached without recursion, and skipping an unknown struct field
// costs O(1) regardless of its size.
//
// Offsets are uint32, which keeps an item at 24 bytes; MaxEncodedBytes is
// capped at math.MaxInt32, and an arena never holds more items than bytes.
type item struct {
	argument uint64 // integer value, length, count, tag number, simple value or float bits
	start    uint32 // offset of the item's initial byte
	end      uint32 // offset just past the item
	next     uint32 // arena index of the next sibling
	major    byte
	minor    byte // additional information
}

func (value *item) isFloat() bool {
	return value.major == 7 && value.minor >= 25 && value.minor <= 27
}

func (value *item) float() float64 {
	switch value.minor {
	case 25:
		return halfFloat(uint16(value.argument))
	case 26:
		return float64(math.Float32frombits(uint32(value.argument)))
	}
	return math.Float64frombits(value.argument)
}

func (value *item) isNull() bool { return value.major == 7 && value.minor == 22 }

// payload is the offset of a string's content.
func (value *item) payload() int { return int(value.end) - int(value.argument) }

// parser reads one item from input or, when reader is set, from a stream
// without read-ahead. data holds every byte of the current item consumed so
// far; in slice mode it is a prefix of input and is never copied.
type parser struct {
	reader  io.Reader
	input   []byte
	options DecoderOptions
	data    []byte
	items   []item
}

// maxPooledItems bounds the arena kept for reuse, so one large item does not
// pin memory for the life of the process.
const maxPooledItems = 4096

var arenaPool = sync.Pool{New: func() any { return new([]item) }}

func getArena() *[]item { return arenaPool.Get().(*[]item) }

func putArena(arena *[]item, items []item) {
	if cap(items) <= maxPooledItems {
		*arena = items[:0]
		arenaPool.Put(arena)
	}
}

func (parser *parser) read(count int) ([]byte, error) {
	start := len(parser.data)
	if parser.reader == nil && uint(count) <= uint(len(parser.input)-start) && count <= parser.options.MaxEncodedBytes-start {
		parser.data = parser.input[:start+count]
		return parser.data[start:], nil
	}
	return parser.readSlow(start, count)
}

func (parser *parser) readSlow(start, count int) ([]byte, error) {
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

// grow doubles the arena, rather than using append's gentler growth for
// large slices, so the copies made while growing total at most the final
// size. In slice mode, every item needs at least one byte, so the arena never
// grows past the items the remaining input could hold.
func (parser *parser) grow() {
	size := max(2*cap(parser.items), 64)
	if parser.reader == nil {
		size = min(size, len(parser.items)+len(parser.input)-len(parser.data)+1)
	}
	parser.items = append(make([]item, 0, size), parser.items...)
}

// header reads an initial byte and its argument.
func (parser *parser) header(result *item) error {
	start := len(parser.data)
	var initial byte
	if parser.reader == nil && start < len(parser.input) && start < parser.options.MaxEncodedBytes {
		// Fast path for slices: no bounds bookkeeping beyond the checks above.
		initial = parser.input[start]
		parser.data = parser.input[:start+1]
	} else {
		encoded, err := parser.read(1)
		if err != nil {
			return err
		}
		initial = encoded[0]
	}
	result.start = uint32(start)
	result.major = initial >> 5
	result.minor = initial & 31
	result.argument = uint64(result.minor)
	switch {
	case result.minor < 24:
		return nil
	case result.minor == 31:
		if result.major >= 2 && result.major <= 5 {
			return newError(ErrUnsupported, start, "indefinite-length item")
		}
		return newError(ErrMalformed, start, "unexpected break or indefinite-length header")
	case result.minor >= 28:
		return newError(ErrMalformed, start, "reserved additional information")
	}
	width := 1 << (result.minor - 24)
	encoded, err := parser.read(width)
	if err != nil {
		return err
	}
	result.argument = 0
	for _, octet := range encoded {
		result.argument = result.argument<<8 | uint64(octet)
	}
	if result.major != 7 && parser.options.RejectNonMinimal && !minimal(result.argument, width) {
		return newError(ErrNotDeterministic, start, "non-minimal argument")
	}
	return nil
}

// parse records one item, and its subtree, in the arena and returns its index.
func (parser *parser) parse(depth int) (int, error) {
	if depth > parser.options.MaxNestedLevels {
		return 0, newError(ErrLimit, len(parser.data), "nesting limit exceeded")
	}
	index := len(parser.items)
	if index == cap(parser.items) {
		parser.grow()
	}
	// header assigns start, major, minor and argument, and end and next are
	// assigned below, so the reused slot needs no zeroing.
	parser.items = parser.items[:index+1]
	result := &parser.items[index]
	if err := parser.header(result); err != nil {
		return 0, err
	}
	switch result.major {
	case 2, 3:
		limit := parser.options.MaxBytes
		if result.major == 3 {
			limit = parser.options.MaxStringBytes
		}
		if result.argument > uint64(limit) {
			return 0, newError(ErrLimit, result.start, "string size limit exceeded")
		}
		payload := len(parser.data)
		encoded, err := parser.read(int(result.argument))
		if err != nil {
			return 0, err
		}
		if result.major == 3 && !utf8.Valid(encoded) {
			return 0, newError(ErrInvalid, payload, "invalid UTF-8")
		}
	case 4, 5:
		if err := parser.container(index, depth); err != nil {
			return 0, err
		}
		result = &parser.items[index] // the arena may have moved
	case 6:
		if parser.options.RejectTags {
			return 0, newError(ErrUnsupported, result.start, "tags rejected by decoder options")
		}
		if _, err := parser.parse(depth + 1); err != nil {
			return 0, err
		}
		result = &parser.items[index]
	case 7:
		switch {
		case result.minor == 24 && result.argument < 32:
			return 0, newError(ErrMalformed, result.start, "two-byte simple value below 32")
		case result.isFloat() && parser.options.RequireDeterministic:
			if minor, bits := shortestFloat(result.float()); minor != result.minor || bits != result.argument {
				return 0, newError(ErrNotDeterministic, result.start, "floating-point value not in shortest form")
			}
		}
	}
	result.end = uint32(len(parser.data))
	result.next = uint32(len(parser.items))
	return index, nil
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

func (parser *parser) container(index, depth int) error {
	result := parser.items[index]
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
	// Every child occupies at least one byte, so a count beyond the remaining
	// budget fails before any child is read.
	if count > uint64(parser.options.MaxEncodedBytes-len(parser.data)) {
		return newError(ErrLimit, result.start, "container exceeds encoded size limit")
	}
	if result.major == 4 {
		for range count {
			if _, err := parser.parse(depth + 1); err != nil {
				return err
			}
		}
		return nil
	}
	var keys keySet
	previous := -1
	for range result.argument {
		key, err := parser.parse(depth + 1)
		if err != nil {
			return err
		}
		if err := keys.add(parser.data, parser.items, key); err != nil {
			return err
		}
		if parser.options.RequireDeterministic && previous >= 0 {
			before, current := &parser.items[previous], &parser.items[key]
			if bytes.Compare(parser.data[before.start:before.end], parser.data[current.start:current.end]) >= 0 {
				return newError(ErrNotDeterministic, current.start, "map keys not in bytewise lexicographic order")
			}
		}
		previous = key
		if _, err := parser.parse(depth + 1); err != nil {
			return err
		}
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

// keySet detects duplicate map keys. Small maps keep a fingerprint and arena
// index per key on the stack and compare keys in full only when
// fingerprints match; larger maps switch to a hash set.
type keySet struct {
	count     int // keys added so far
	ids       map[keyID]struct{}
	prints    [linearKeyScan]uint64
	positions [linearKeyScan]uint32
}

const linearKeyScan = 16

// fingerprint is equal for keys that are equal in the CBOR data model, so
// unequal fingerprints prove keys distinct. Integers and simple values use
// their major type and argument, strings also their first bytes, and floats
// their value with -0 folded into +0.
func fingerprint(data []byte, key *item) uint64 {
	switch {
	case key.major == 2 || key.major == 3:
		print := uint64(key.major)<<61 ^ key.argument
		for _, octet := range data[key.payload():min(key.payload()+8, int(key.end))] {
			print = print<<8 ^ print>>56 ^ uint64(octet)
		}
		return print
	case key.isFloat():
		number := key.float()
		if number == 0 {
			number = 0
		}
		return math.Float64bits(number) ^ 0x9e3779b97f4a7c15 // distinguish from integer prints
	}
	return uint64(key.major)<<61 ^ key.argument
}

func (keys *keySet) add(data []byte, items []item, index int) error {
	key := &items[index]
	if err := checkKeyType(key); err != nil {
		return err
	}
	if keys.count < linearKeyScan {
		print := fingerprint(data, key)
		for position, previous := range keys.prints[:keys.count] {
			if previous == print && sameKey(data, &items[keys.positions[position]], key) {
				return newError(ErrInvalid, key.start, "duplicate map key")
			}
		}
		keys.prints[keys.count], keys.positions[keys.count] = print, uint32(index)
		keys.count++
		return nil
	}
	if keys.ids == nil {
		keys.ids = make(map[keyID]struct{}, 2*linearKeyScan)
		for _, position := range keys.positions {
			keys.ids[identify(data, &items[position])] = struct{}{}
		}
	}
	id := identify(data, key)
	if _, ok := keys.ids[id]; ok {
		return newError(ErrInvalid, key.start, "duplicate map key")
	}
	keys.ids[id] = struct{}{}
	keys.count++
	return nil
}

func checkKeyType(key *item) error {
	if key.major == 4 || key.major == 5 || key.major == 6 {
		return newError(ErrUnsupported, key.start, "composite map keys unsupported")
	}
	if key.isFloat() && math.IsNaN(key.float()) {
		return newError(ErrUnsupported, key.start, "NaN map keys unsupported")
	}
	return nil
}

func sameKey(data []byte, left, right *item) bool {
	if left.isFloat() || right.isFloat() {
		return left.isFloat() && right.isFloat() && left.float() == right.float()
	}
	if left.major != right.major || left.argument != right.argument {
		return false
	}
	if left.major == 2 || left.major == 3 {
		return bytes.Equal(data[left.payload():left.end], data[right.payload():right.end])
	}
	return true
}

func identify(data []byte, key *item) keyID {
	switch {
	case key.isFloat():
		number := key.float()
		if number == 0 {
			number = 0 // -0 and +0 compare equal, as with sameKey.
		}
		return keyID{kind: 8, argument: math.Float64bits(number)}
	case key.major == 2 || key.major == 3:
		return keyID{kind: key.major, argument: key.argument, text: string(data[key.payload():key.end])}
	}
	return keyID{kind: key.major, argument: key.argument}
}
