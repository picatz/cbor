package cbor

import (
	"reflect"
	"strings"
	"sync"
)

// structTypeCache is a cache of struct types used to reduce allocations
// when decoding CBOR into structs, avoiding the need to reflect on the
// struct type for each field.
var structTypeCache sync.Map

// storeFieldCache adds a struct type to the cache from the given reflect.Value
// if it is not already in the cache.
func storeFieldCache(rv reflect.Value) fieldCache {
	// Check if the type is already in the cache.
	t := rv.Type()

	if v, ok := structTypeCache.Load(t); ok {
		fc, ok := v.(fieldCache)
		if !ok {
			panic("cbor: invalid field cache")
		}
		return fc
	}

	fieldCache := make(fieldCache, rv.NumField())

	// Iterate over the map fields in the struct to build
	// a cache of field names and keyasint values.
	for i := 0; i < rv.NumField(); i++ {
		field := rv.Type().Field(i)

		// If the field is unexported, skip it.
		if field.PkgPath != "" {
			continue
		}

		// If the field has no cbor tag, add it to the
		// field name cache with the field name as the key.
		if field.Tag == "" {
			fieldCache[field.Name] = rv.Field(i)
			continue
		}

		// Check cbor tag for keyasint.
		if tag := field.Tag.Get("cbor"); tag != "" {
			// Use index to avoid allocating a new string.
			if idx := strings.Index(tag, ",keyasint"); idx != -1 {
				// If the tag is "keyasint", add it to the field cache.
				fieldCache[tag[:idx]] = rv.Field(field.Index[0])
			} else {
				// If the tag is not "keyasint", add it to the field cache
				// with the tag value as the key.
				fieldCache[tag] = rv.Field(field.Index[0])
			}
		}
	}

	structTypeCache.Store(t, fieldCache)

	return fieldCache
}

// loadFieldCache returns the field cache for the given struct type, or nil
// if the type is not in the cache.
func loadFieldCache(t reflect.Type) fieldCache {
	if v, ok := structTypeCache.Load(t); ok {
		return v.(fieldCache)
	}

	return nil
}
