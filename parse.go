package cbor

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"unicode/utf8"
)

// item is the header of one data item, read and validated by parser.item.
// Scalars stay in their encoded form so reading allocates nothing per value.
//
// For integers, strings, simple values and floats, end is the offset just
// past the item. For arrays, maps and tags, skipRest assigns it once the
// children have been consumed; a decoder that reads the children itself
// does not need it.
type item struct {
	argument uint64 // integer value, length, count, tag number, simple value or float bits
	start    int    // offset of the item's initial byte
	end      int    // offset just past the item, once known
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
func (value *item) payload() int { return value.end - int(value.argument) }

// parser reads items from a slice or, when reader is set, from a stream
// without read-ahead. data holds the bytes available, of which the first
// pos have been consumed: in slice mode it is the input itself and is never
// copied, and in stream mode it grows by appending as items are read, so
// pos is its length.
//
// Decoding is a single pass: each item is validated as it is read, and
// everything observable, such as calls into application code, waits until
// the whole item has been validated. Slices of data must not be retained
// across reads, since in stream mode appending may move it; offsets are
// stable.
type parser struct {
	reader  io.Reader
	options *DecoderOptions // normalized, and never modified
	data    []byte
	pos     int

	// limit is the offset up to which data can be read without checks, in
	// slice mode: the end of the input or of the encoded size budget. It
	// is zero in stream mode, where pos is already the end of data.
	limit int
}

// setInput prepares the parser to read one item from data.
func (parser *parser) setInput(data []byte) {
	parser.data, parser.pos = data, 0
	parser.limit = min(len(data), parser.options.MaxEncodedBytes)
}

func (parser *parser) read(count int) ([]byte, error) {
	start := parser.pos
	if parser.reader == nil && uint(count) <= uint(len(parser.data)-start) && count <= parser.options.MaxEncodedBytes-start {
		parser.pos = start + count
		return parser.data[start:parser.pos], nil
	}
	return parser.readSlow(start, count)
}

func (parser *parser) readSlow(start, count int) ([]byte, error) {
	if count < 0 || count > parser.options.MaxEncodedBytes-start {
		return nil, newError(ErrLimit, start, "encoded size limit exceeded")
	}
	if parser.reader == nil {
		if count > len(parser.data)-start {
			if start == 0 && len(parser.data) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		}
		parser.pos = start + count
		return parser.data[start:parser.pos], nil
	}
	parser.data = append(parser.data, make([]byte, count)...)
	read, err := io.ReadFull(parser.reader, parser.data[start:])
	if err != nil {
		parser.data = parser.data[:start+read]
		parser.pos = len(parser.data)
		if err == io.EOF && start > 0 {
			err = io.ErrUnexpectedEOF
		}
		return nil, err
	}
	parser.pos = len(parser.data)
	return parser.data[start:], nil
}

// item reads and validates one item's header at nesting depth, and the
// content of a string. Every limit is checked here, before the caller
// allocates for the item or descends into its children. The children of an
// array, map or tag are left for the caller to read, with skipRest or one
// item at a time.
//
// One-byte items (integers below 24, false, true, null and undefined), the
// usual map keys of COSE and CWT, are read inline; they need no check
// beyond the nesting limit.
func (parser *parser) item(result *item, depth int) error {
	start := parser.pos
	if start < parser.limit && depth <= parser.options.MaxNestedLevels {
		if initial := parser.data[start]; initial&31 < 24 && (initial < 0x40 || initial >= 0xf4) {
			*result = item{argument: uint64(initial & 31), start: start, end: start + 1, major: initial >> 5, minor: initial & 31}
			parser.pos = start + 1
			return nil
		}
	}
	return parser.itemSlow(result, depth)
}

// itemSlow is item for every other input.
func (parser *parser) itemSlow(result *item, depth int) error {
	start := parser.pos
	if depth > parser.options.MaxNestedLevels {
		return newError(ErrLimit, start, "nesting limit exceeded")
	}
	var initial byte
	if start < parser.limit {
		// Fast path for slices: no bounds bookkeeping beyond the check above.
		initial = parser.data[start]
		parser.pos = start + 1
	} else {
		encoded, err := parser.read(1)
		if err != nil {
			return err
		}
		initial = encoded[0]
	}
	result.start = start
	result.major = initial >> 5
	result.minor = initial & 31
	result.argument = uint64(result.minor)
	if result.minor >= 24 {
		width := 1 << (result.minor - 24) // 16 and 32 for the reserved values
		if result.minor < 28 && width <= parser.limit-start-1 {
			// Fast path for slices, with the same checks as read.
			encoded := parser.data[start+1 : start+1+width]
			parser.pos = start + 1 + width
			switch width {
			case 1:
				result.argument = uint64(encoded[0])
			case 2:
				result.argument = uint64(binary.BigEndian.Uint16(encoded))
			case 4:
				result.argument = uint64(binary.BigEndian.Uint32(encoded))
			default:
				result.argument = binary.BigEndian.Uint64(encoded)
			}
			if result.major != 7 && parser.options.RejectNonMinimal && !minimal(result.argument, width) {
				return newError(ErrNotDeterministic, start, "non-minimal argument")
			}
		} else if err := parser.argument(result); err != nil {
			return err
		}
	}
	switch result.major {
	case 2, 3:
		limit := parser.options.MaxBytes
		if result.major == 3 {
			limit = parser.options.MaxStringBytes
		}
		if result.argument > uint64(limit) {
			return newError(ErrLimit, result.start, "string size limit exceeded")
		}
		payload, length := parser.pos, int(result.argument)
		var encoded []byte
		if length <= parser.limit-payload {
			encoded = parser.data[payload : payload+length]
			parser.pos = payload + length
		} else {
			var err error
			if encoded, err = parser.read(length); err != nil {
				return err
			}
		}
		if result.major == 3 && !utf8.Valid(encoded) {
			return newError(ErrInvalid, payload, "invalid UTF-8")
		}
	case 4, 5:
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
		// Every child occupies at least one byte, so a count beyond the
		// remaining budget fails before any child is read.
		if count > uint64(parser.options.MaxEncodedBytes-parser.pos) {
			return newError(ErrLimit, result.start, "container exceeds encoded size limit")
		}
		// Decoders allocate a container's elements before reading them, so
		// in slice mode a count the remaining input cannot hold fails here,
		// keeping every allocation proportional to bytes actually present.
		if parser.reader == nil && count > uint64(len(parser.data)-parser.pos) {
			return io.ErrUnexpectedEOF
		}
	case 6:
		if parser.options.RejectTags {
			return newError(ErrUnsupported, result.start, "tags rejected by decoder options")
		}
	case 7:
		switch {
		case result.minor == 24 && result.argument < 32:
			return newError(ErrMalformed, result.start, "two-byte simple value below 32")
		case result.isFloat() && parser.options.RequireDeterministic:
			if minor, bits := shortestFloat(result.float()); minor != result.minor || bits != result.argument {
				return newError(ErrNotDeterministic, result.start, "floating-point value not in shortest form")
			}
		}
	}
	result.end = parser.pos
	return nil
}

// argument reads the argument of a header whose additional information is
// 24 or more: a one- to eight-byte integer, or a reserved value.
func (parser *parser) argument(result *item) error {
	switch {
	case result.minor == 31:
		if result.major >= 2 && result.major <= 5 {
			return newError(ErrUnsupported, result.start, "indefinite-length item")
		}
		return newError(ErrMalformed, result.start, "unexpected break or indefinite-length header")
	case result.minor >= 28:
		return newError(ErrMalformed, result.start, "reserved additional information")
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
		return newError(ErrNotDeterministic, result.start, "non-minimal argument")
	}
	return nil
}

// ordered checks that key follows the key at previous in bytewise
// lexicographic order of their encodings, as RequireDeterministic demands.
// Deterministic encoding gives equal keys equal encodings, so an equal
// encoding is reported as the duplicate it is.
func (parser *parser) ordered(previous int, key *item) error {
	before := readItem(parser.data, previous)
	switch bytes.Compare(parser.data[before.start:before.end], parser.data[key.start:key.end]) {
	case 0:
		return newError(ErrInvalid, key.start, "duplicate map key")
	case 1:
		return newError(ErrNotDeterministic, key.start, "map keys not in bytewise lexicographic order")
	}
	return nil
}

// readKey reads a map key and checks its type and, after the key at
// previous (-1 for the first), its order. Duplicates are left to the
// caller, which detects them when it stores the key.
func (parser *parser) readKey(key *item, previous, depth int) error {
	if err := parser.item(key, depth); err != nil {
		return err
	}
	if err := checkKeyType(key); err != nil {
		return err
	}
	if parser.options.RequireDeterministic && previous >= 0 {
		return parser.ordered(previous, key)
	}
	return nil
}

// skip reads and validates one whole item, without decoding it.
func (parser *parser) skip(result *item, depth int) error {
	if err := parser.item(result, depth); err != nil {
		return err
	}
	return parser.skipRest(result, depth)
}

// skipRest reads and validates the children of an item read by item, and
// records where the item ends.
func (parser *parser) skipRest(value *item, depth int) error {
	switch value.major {
	case 4:
		var child item
		for range value.argument {
			if err := parser.skip(&child, depth+1); err != nil {
				return err
			}
		}
	case 5:
		var keys keySet
		var key, child item
		for range value.argument {
			if err := parser.key(&keys, &key, depth+1); err != nil {
				return err
			}
			if err := parser.skip(&child, depth+1); err != nil {
				return err
			}
		}
	case 6:
		var child item
		if err := parser.skip(&child, depth+1); err != nil {
			return err
		}
	}
	value.end = parser.pos
	return nil
}

// key reads and validates a map key: it must be a scalar other than NaN,
// distinct from the map's earlier keys, which keys holds, and under
// RequireDeterministic after them in bytewise lexicographic order.
func (parser *parser) key(keys *keySet, key *item, depth int) error {
	previous := -1
	if keys.count > 0 {
		previous = keys.last
	}
	if err := parser.readKey(key, previous, depth); err != nil {
		return err
	}
	return keys.add(parser.data, key)
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

// readItem re-reads the header of a scalar item that item already
// validated at start.
func readItem(data []byte, start int) item {
	initial := data[start]
	value := item{start: start, major: initial >> 5, minor: initial & 31}
	value.argument = uint64(value.minor)
	value.end = start + 1
	if value.minor >= 24 {
		value.argument = 0
		for _, octet := range data[value.end : value.end+1<<(value.minor-24)] {
			value.argument = value.argument<<8 | uint64(octet)
		}
		value.end += 1 << (value.minor - 24)
	}
	if value.major == 2 || value.major == 3 {
		value.end += int(value.argument)
	}
	return value
}

// keyID identifies a map key in the CBOR data model, independent of argument
// width: integers by value, strings by content, floats by numeric value.
type keyID struct {
	kind     byte // major type, or 8 for floating-point values
	argument uint64
	text     string
}

// keySet detects duplicate map keys. Small maps keep a fingerprint and
// offset per key on the stack and compare keys in full only when
// fingerprints match; larger maps switch to a hash set.
type keySet struct {
	count  int // keys added so far
	last   int // offset of the last key added
	ids    map[keyID]struct{}
	prints [linearKeyScan]uint64
	starts [linearKeyScan]int
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
		for _, octet := range data[key.payload():min(key.payload()+8, key.end)] {
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

func (keys *keySet) add(data []byte, key *item) error {
	keys.last = key.start
	if keys.count < linearKeyScan {
		print := fingerprint(data, key)
		for position, previous := range keys.prints[:keys.count] {
			if previous == print {
				before := readItem(data, keys.starts[position])
				if sameKey(data, &before, key) {
					return newError(ErrInvalid, key.start, "duplicate map key")
				}
			}
		}
		keys.prints[keys.count], keys.starts[keys.count] = print, key.start
		keys.count++
		return nil
	}
	if keys.ids == nil {
		keys.ids = make(map[keyID]struct{}, 2*linearKeyScan)
		for _, start := range keys.starts {
			before := readItem(data, start)
			keys.ids[identify(data, &before)] = struct{}{}
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

// checkKeyType rejects composite and NaN keys. Integer and string keys, the
// common case, pass without a call.
func checkKeyType(key *item) error {
	if key.major <= 3 {
		return nil
	}
	return checkKeyTypeSlow(key)
}

func checkKeyTypeSlow(key *item) error {
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
