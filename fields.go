package cbor

import (
	"bytes"
	"cmp"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// structInfo describes how a struct type maps to CBOR. It is computed once
// per type and holds no reference to decoded or encoded values.
type structInfo struct {
	// fields are in declaration order, with promoted fields of embedded
	// structs in place of the embedded field, as in encoding/json.
	fields []structField

	// order lists field positions sorted by encoded key for each SortMode,
	// so encoding a struct never sorts at run time.
	order [2][]int

	// toArray encodes the struct as an array of its fields in declaration
	// order instead of a map, as COSE and CWT structures require. It is set
	// by a blank field tagged `cbor:",toarray"`.
	toArray bool

	// required counts fields tagged required.
	required int

	// Lookup tables for structs with many fields. Smaller structs are
	// searched linearly, starting where the previous key matched.
	byText  map[string]int
	byUint  map[uint64]int
	byNeg   map[uint64]int
	hasText bool // some field has a text key, for case-insensitive matching
}

// structField is one encoded field, possibly promoted from an embedded struct.
type structField struct {
	goName     string       // Go field name, for error messages
	index      []int        // reflect index path, through embedded structs
	typ        reflect.Type // field type
	keyKind    byte         // 0 (unsigned), 1 (negative) or 3 (text)
	number     uint64       // CBOR argument for integer keys
	name       string       // text key
	encodedKey []byte       // preferred serialization of the key
	omitEmpty  bool
	omitZero   bool
	zeroer     zeroer // how omitzero tests the field, when it has IsZero
	required   bool
	tagged     bool // the key was named in a struct tag
}

// manyFields is the field count above which lookups use hash tables.
const manyFields = 12

type cachedStruct struct {
	info *structInfo
	err  error
}

// structCache maps a struct reflect.Type to its cachedStruct.
var structCache sync.Map

// structFor returns the CBOR mapping of a struct type.
func structFor(kind reflect.Type) (*structInfo, error) {
	if cached, ok := structCache.Load(kind); ok {
		entry := cached.(cachedStruct)
		return entry.info, entry.err
	}
	info, err := computeStruct(kind)
	structCache.Store(kind, cachedStruct{info: info, err: err})
	return info, err
}

func structError(kind reflect.Type, msg string) error {
	return encodeError(ErrUnsupported, kind.String()+": "+msg)
}

// structOptions are set on a blank field: _ struct{} `cbor:",toarray"`.
type structOptions struct {
	toArray  bool // encode as an array of fields
	keyAsInt bool // every field name is an integer key
}

func optionsOf(kind reflect.Type) (structOptions, error) {
	var options structOptions
	for index := range kind.NumField() {
		field := kind.Field(index)
		if field.Name != "_" {
			continue
		}
		tag, ok := field.Tag.Lookup("cbor")
		if !ok {
			continue
		}
		name, list, _ := strings.Cut(tag, ",")
		if name != "" {
			return options, structError(kind, "blank field takes only options, such as `cbor:\",toarray\"`")
		}
		for option := range strings.SplitSeq(list, ",") {
			switch option {
			case "toarray":
				options.toArray = true
			case "keyasint":
				options.keyAsInt = true
			case "":
			default:
				return options, structError(kind, "unsupported blank field option "+strconv.Quote(option))
			}
		}
	}
	if options.toArray && options.keyAsInt {
		return options, structError(kind, "toarray structs have no keys, so keyasint does not apply")
	}
	return options, nil
}

// computeStruct applies the encoding/json field rules: exported fields, and
// fields promoted from embedded structs (pointer or not), with shallower
// fields dominating deeper ones as Go's selector rules do. Unlike
// encoding/json, two fields with one key at the same depth are an error
// rather than a tie broken by tags or a silently dropped field.
func computeStruct(kind reflect.Type) (*structInfo, error) {
	top, err := optionsOf(kind)
	if err != nil {
		return nil, err
	}
	type embedded struct {
		kind  reflect.Type
		index []int
	}
	var (
		fields        []structField
		current, next = []embedded{}, []embedded{{kind: kind}}
		count         = map[reflect.Type]int{}
		nextCount     = map[reflect.Type]int{}
		visited       = map[reflect.Type]bool{}
	)
	for len(next) > 0 {
		current, next = next, current[:0]
		count, nextCount = nextCount, map[reflect.Type]int{}
		for _, parent := range current {
			if visited[parent.kind] {
				continue
			}
			visited[parent.kind] = true
			options := top
			if parent.index != nil {
				if options, err = optionsOf(parent.kind); err != nil {
					return nil, err
				}
				if options.toArray {
					return nil, structError(kind, "cannot flatten embedded toarray struct "+parent.kind.String()+"; give the field a name")
				}
			}
			for position := range parent.kind.NumField() {
				field := parent.kind.Field(position)
				if field.Name == "_" {
					continue
				}
				target := field.Type
				if target.Name() == "" && target.Kind() == reflect.Pointer {
					target = target.Elem()
				}
				if field.Anonymous {
					if !field.IsExported() && (target.Kind() != reflect.Struct || field.Type.Kind() == reflect.Pointer) {
						// Unexported non-struct types have no fields to promote,
						// and an unexported pointer cannot be allocated.
						continue
					}
				} else if !field.IsExported() {
					continue
				}
				tag := field.Tag.Get("cbor")
				if tag == "-" {
					continue
				}
				name, list, _ := strings.Cut(tag, ",")
				index := append(slices.Clip(parent.index), position)
				if name == "" && field.Anonymous && target.Kind() == reflect.Struct {
					if list != "" {
						return nil, structError(kind, "options on embedded struct "+field.Name+" need a field name")
					}
					if top.toArray && field.Type.Kind() == reflect.Pointer {
						return nil, structError(kind, "toarray structs cannot embed pointers, which may be nil")
					}
					// A type reached through a parent embedded several times at
					// this depth is reached that many times, so conflicts
					// among its fields are detected however deep they are.
					if nextCount[target] == 0 {
						next = append(next, embedded{kind: target, index: index})
					}
					nextCount[target] += max(count[parent.kind], 1)
					continue
				}
				metadata, err := newField(kind, options, field, name, list, index)
				if err != nil {
					return nil, err
				}
				fields = append(fields, metadata)
				if count[parent.kind] > 1 {
					// The same type is embedded more than once at this depth,
					// so none of its fields dominates; see resolve.
					fields = append(fields, metadata)
				}
			}
		}
	}
	if fields, err = resolve(kind, fields); err != nil {
		return nil, err
	}
	info := &structInfo{fields: fields, toArray: top.toArray}
	for position := range fields {
		field := &fields[position]
		if field.required {
			info.required++
		}
		if field.keyKind == 3 {
			info.hasText = true
		}
		if top.toArray && (field.omitEmpty || field.omitZero) {
			return nil, structError(kind, "toarray structs cannot omit fields")
		}
	}
	for mode := range info.order {
		order := make([]int, len(fields))
		for position := range order {
			order[position] = position
		}
		slices.SortFunc(order, func(left, right int) int {
			return compareKeys(SortMode(mode), fields[left].encodedKey, fields[right].encodedKey)
		})
		info.order[mode] = order
	}
	if len(fields) > manyFields {
		info.byText, info.byUint, info.byNeg = map[string]int{}, map[uint64]int{}, map[uint64]int{}
		for position, field := range fields {
			switch field.keyKind {
			case 0:
				info.byUint[field.number] = position
			case 1:
				info.byNeg[field.number] = position
			default:
				info.byText[field.name] = position
			}
		}
	}
	return info, nil
}

// newField parses one field's tag. options are those of the struct that
// declares the field, so a promoted field keeps its own struct's keyasint.
func newField(kind reflect.Type, options structOptions, field reflect.StructField, name, list string, index []int) (structField, error) {
	metadata := structField{goName: field.Name, index: index, typ: field.Type, keyKind: 3, name: name, tagged: name != ""}
	if name == "" {
		metadata.name = field.Name
	}
	keyAsInt := options.keyAsInt
	for option := range strings.SplitSeq(list, ",") {
		switch option {
		case "keyasint":
			keyAsInt = true
		case "omitempty":
			metadata.omitEmpty = true
		case "omitzero":
			metadata.omitZero = true
			metadata.zeroer = zeroerFor(field.Type)
		case "required":
			metadata.required = true
		case "toarray":
			return metadata, structError(kind, "toarray belongs on a blank field: _ struct{} `cbor:\",toarray\"`")
		case "":
		default:
			return metadata, structError(kind, "unsupported option "+strconv.Quote(option)+" on field "+field.Name)
		}
	}
	if metadata.required && (metadata.omitEmpty || metadata.omitZero) {
		return metadata, structError(kind, "required field "+field.Name+" cannot also be omitted")
	}
	if keyAsInt {
		if !metadata.tagged {
			return metadata, structError(kind, "field "+field.Name+" needs an integer key, such as `cbor:\"1\"`")
		}
		number, err := strconv.ParseInt(name, 10, 64)
		switch {
		case strings.HasPrefix(name, "+"):
			return metadata, structError(kind, "field "+field.Name+" has invalid integer key "+strconv.Quote(name))
		case err == nil && number < 0:
			metadata.keyKind, metadata.number = 1, uint64(-1-number)
		case err == nil:
			metadata.keyKind, metadata.number = 0, uint64(number)
		default:
			unsigned, err := strconv.ParseUint(name, 10, 64)
			if err != nil {
				return metadata, structError(kind, "field "+field.Name+" has invalid integer key "+strconv.Quote(name))
			}
			metadata.keyKind, metadata.number = 0, unsigned
		}
		metadata.encodedKey = appendHeader(nil, metadata.keyKind, metadata.number)
		return metadata, nil
	}
	if !utf8.ValidString(metadata.name) {
		return metadata, structError(kind, "invalid UTF-8 field key")
	}
	metadata.encodedKey = append(appendHeader(nil, 3, uint64(len(metadata.name))), metadata.name...)
	return metadata, nil
}

// resolve keeps, for each key, the shallowest field, and returns the result
// in declaration order.
func resolve(kind reflect.Type, fields []structField) ([]structField, error) {
	slices.SortStableFunc(fields, func(left, right structField) int {
		return cmp.Or(
			bytes.Compare(left.encodedKey, right.encodedKey),
			cmp.Compare(len(left.index), len(right.index)),
		)
	})
	resolved := fields[:0]
	for start := 0; start < len(fields); {
		end := start + 1
		for end < len(fields) && bytes.Equal(fields[end].encodedKey, fields[start].encodedKey) {
			end++
		}
		group := fields[start:end]
		if len(group) > 1 && len(group[0].index) == len(group[1].index) {
			return nil, structError(kind, "fields "+group[0].goName+" and "+group[1].goName+" have the same key "+keyString(group[0]))
		}
		resolved = append(resolved, group[0])
		start = end
	}
	slices.SortFunc(resolved, func(left, right structField) int { return slices.Compare(left.index, right.index) })
	return resolved, nil
}

func keyString(field structField) string {
	switch field.keyKind {
	case 0:
		return strconv.FormatUint(field.number, 10)
	case 1:
		if field.number == 1<<63-1 {
			return "-9223372036854775808"
		}
		return "-" + strconv.FormatUint(field.number+1, 10)
	}
	return strconv.Quote(field.name)
}

// lookup returns the position of the field whose key equals key, trying hint
// first because keys usually arrive in field order. It returns -1 when no
// field matches.
func (info *structInfo) lookup(data []byte, key *item, hint int) int {
	if key.major != 0 && key.major != 1 && key.major != 3 {
		return -1
	}
	if info.byText != nil {
		var position int
		var ok bool
		switch key.major {
		case 0:
			position, ok = info.byUint[key.argument]
		case 1:
			position, ok = info.byNeg[key.argument]
		default:
			position, ok = info.byText[string(data[key.payload():key.end])]
		}
		if !ok {
			return -1
		}
		return position
	}
	fields := info.fields
	for offset := range fields {
		position := hint + offset
		if position >= len(fields) {
			position -= len(fields)
		}
		if fields[position].matches(data, key) {
			return position
		}
	}
	return -1
}

func (field *structField) matches(data []byte, key *item) bool {
	if key.major != field.keyKind {
		return false
	}
	if field.keyKind == 3 {
		return string(data[key.payload():key.end]) == field.name
	}
	return key.argument == field.number
}

// fold returns the single text-keyed field whose name equals key under
// Unicode case folding, -1 when there is none, or -2 when several match.
func (info *structInfo) fold(data []byte, key *item) int {
	if !info.hasText || key.major != 3 {
		return -1
	}
	name := data[key.payload():key.end]
	selected := -1
	for position := range info.fields {
		if info.fields[position].keyKind == 3 && bytes.EqualFold([]byte(info.fields[position].name), name) {
			if selected >= 0 {
				return -2
			}
			selected = position
		}
	}
	return selected
}

// zeroer records how a type reports zero for omitzero: through an
// IsZero() bool method on the type or on its pointer, or by its Go zero
// value. It is computed once per field.
type zeroer uint8

const (
	zeroByValue   zeroer = iota // no IsZero method: compare with the zero value
	zeroByMethod                // the type has IsZero
	zeroByPointer               // only *T has IsZero, usable when addressable
)

// isZeroer is implemented by types such as time.Time.
type isZeroer interface{ IsZero() bool }

var isZeroerType = reflect.TypeFor[isZeroer]()

func zeroerFor(kind reflect.Type) zeroer {
	switch {
	case kind.Implements(isZeroerType):
		return zeroByMethod
	case kind.Kind() != reflect.Pointer && kind.Kind() != reflect.Interface && reflect.PointerTo(kind).Implements(isZeroerType):
		return zeroByPointer
	}
	return zeroByValue
}

// omit reports whether an omitempty or omitzero field is left out. An
// omitzero field is omitted when its IsZero method reports true, or, as in
// encoding/json, when it has none and holds its type's zero value.
func (field *structField) omit(value reflect.Value) bool {
	if field.omitEmpty && emptyValue(value) {
		return true
	}
	if !field.omitZero {
		return false
	}
	switch field.zeroer {
	case zeroByMethod:
		if kind := value.Kind(); (kind == reflect.Pointer || kind == reflect.Interface) && value.IsNil() {
			return true
		}
		if value.CanAddr() {
			// Calling through the pointer avoids boxing a copy of the value.
			return value.Addr().Interface().(isZeroer).IsZero()
		}
		return value.Interface().(isZeroer).IsZero()
	case zeroByPointer:
		if value.CanAddr() {
			return value.Addr().Interface().(isZeroer).IsZero()
		}
	}
	return value.IsZero()
}

func emptyValue(value reflect.Value) bool {
	switch value.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return value.Len() == 0
	}
	return value.IsZero()
}

// fieldByIndex returns the field at index for encoding. It reports false
// when an embedded pointer on the path is nil, so the field is absent.
func fieldByIndex(value reflect.Value, index []int) (reflect.Value, bool) {
	if len(index) == 1 {
		return value.Field(index[0]), true
	}
	for position, step := range index {
		if position > 0 && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				return reflect.Value{}, false
			}
			value = value.Elem()
		}
		value = value.Field(step)
	}
	return value, true
}

// fieldForDecode returns the field at index for decoding, allocating nil
// embedded pointers on the path.
func fieldForDecode(value reflect.Value, index []int) reflect.Value {
	if len(index) == 1 {
		return value.Field(index[0])
	}
	for position, step := range index {
		if position > 0 && value.Kind() == reflect.Pointer {
			if value.IsNil() {
				value.Set(reflect.New(value.Type().Elem()))
			}
			value = value.Elem()
		}
		value = value.Field(step)
	}
	return value
}
