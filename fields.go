package cbor

import (
	"errors"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"
)

type cachedFields struct {
	fields []structField
	err    error
}

// fieldCache maps a struct reflect.Type to its cachedFields. Entries describe
// types only, so the cache never retains decoded or encoded values.
var fieldCache sync.Map

// fieldsFor returns the CBOR fields of a struct type in declaration order.
func fieldsFor(kind reflect.Type) ([]structField, error) {
	if cached, ok := fieldCache.Load(kind); ok {
		entry := cached.(cachedFields)
		return entry.fields, entry.err
	}
	fields, err := computeFields(kind)
	fieldCache.Store(kind, cachedFields{fields: fields, err: err})
	return fields, err
}

func computeFields(kind reflect.Type) ([]structField, error) {
	var fields []structField
	seen := make(map[keyID]bool)
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
		fields = append(fields, metadata)
	}
	return fields, nil
}
