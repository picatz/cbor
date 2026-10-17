package cbor

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

// structInfo describes how a struct type maps to CBOR.
type structInfo struct {
	fields []structField // in declaration order

	// toArray encodes the struct as an array of its fields in declaration
	// order instead of a map, as COSE and CWT structures require. It is set
	// by a blank field tagged `cbor:",toarray"`.
	toArray bool
}

type cachedStruct struct {
	info *structInfo
	err  error
}

// structCache maps a struct reflect.Type to its cachedStruct. Entries describe
// types only, so the cache never retains decoded or encoded values.
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

func computeStruct(kind reflect.Type) (*structInfo, error) {
	info := new(structInfo)
	seen := make(map[keyID]bool)
	for index := 0; index < kind.NumField(); index++ {
		field := kind.Field(index)
		if field.Name == "_" {
			switch field.Tag.Get("cbor") {
			case ",toarray":
				info.toArray = true
			case "":
			default:
				return nil, errors.New("cbor: unsupported tag on blank field")
			}
			continue
		}
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
		metadata := structField{index: index, keyKind: 3, name: name}
		for _, option := range tag[1:] {
			switch option {
			case "keyasint":
				if strings.HasPrefix(name, "-") {
					number, err := strconv.ParseInt(name, 10, 64)
					if err != nil {
						return nil, errors.New("cbor: invalid integer field key")
					}
					if number < 0 {
						metadata.keyKind, metadata.number = 1, uint64(-1-number)
					} else {
						metadata.keyKind, metadata.number = 0, 0
					}
				} else {
					number, err := strconv.ParseUint(name, 10, 64)
					if err != nil {
						return nil, errors.New("cbor: invalid integer field key")
					}
					metadata.keyKind, metadata.number = 0, number
				}
			case "omitempty":
				metadata.omitEmpty = true
			case "omitzero":
				metadata.omitZero = true
			case "toarray":
				return nil, errors.New("cbor: toarray belongs on a blank field: _ struct{} `cbor:\",toarray\"`")
			case "":
			default:
				return nil, errors.New("cbor: unsupported field tag option")
			}
		}
		id := keyID{kind: metadata.keyKind, argument: metadata.number}
		if metadata.keyKind == 3 {
			if !utf8.ValidString(name) {
				return nil, errors.New("cbor: invalid UTF-8 field key")
			}
			id = keyID{kind: 3, text: name}
			metadata.encodedKey = append(appendHeader(nil, 3, uint64(len(name))), name...)
		} else {
			metadata.encodedKey = appendHeader(nil, metadata.keyKind, metadata.number)
		}
		if seen[id] {
			return nil, errors.New("cbor: duplicate struct field key")
		}
		seen[id] = true
		info.fields = append(info.fields, metadata)
	}
	if info.toArray {
		for _, field := range info.fields {
			if field.omitEmpty || field.omitZero {
				return nil, errors.New("cbor: toarray structs cannot omit fields")
			}
		}
	}
	return info, nil
}

// omit reports whether an omitempty or omitzero field is left out.
func (field *structField) omit(value reflect.Value) bool {
	if field.omitEmpty && emptyValue(value) {
		return true
	}
	if !field.omitZero {
		return false
	}
	if value.Type().Implements(isZeroerType) {
		if value.Kind() == reflect.Pointer && value.IsNil() {
			return true
		}
		return value.Interface().(interface{ IsZero() bool }).IsZero()
	}
	return value.IsZero()
}

var isZeroerType = reflect.TypeOf((*interface{ IsZero() bool })(nil)).Elem()
