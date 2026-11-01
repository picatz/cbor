package cbor

import (
	"reflect"
	"slices"
)

// Tag is a tagged item whose content has a static Go type, such as a
// COSE_Sign1 message (tag 18) or an epoch time (tag 1). Use TaggedValue when
// the content's type is not known in advance.
//
// Decoding into a Tag requires a tagged item and stores its number, so the
// caller decides which numbers to accept. An untagged item is an ErrType.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.4
type Tag[T any] struct {
	Number  uint64
	Content T
}

// tagEncoder and tagDecoder let the reflection-based encoder and decoder
// handle every instantiation of Tag.
type tagEncoder interface {
	cborTag() (uint64, reflect.Value)
}

type tagDecoder interface {
	cborTagTarget(number uint64) reflect.Value
}

func (tag Tag[T]) cborTag() (uint64, reflect.Value) {
	return tag.Number, reflect.ValueOf(&tag.Content).Elem()
}

func (tag *Tag[T]) cborTagTarget(number uint64) reflect.Value {
	tag.Number = number
	return reflect.ValueOf(&tag.Content).Elem()
}

var (
	tagEncoderType = reflect.TypeOf((*tagEncoder)(nil)).Elem()
	tagDecoderType = reflect.TypeOf((*tagDecoder)(nil)).Elem()
)

// tagFields returns the index paths of the Number and Content fields of
// kind, which is a Tag instantiation or a struct that embeds exactly one at
// its shallowest embedding depth, as method promotion requires. Paths may
// pass through embedded pointers.
func tagFields(kind reflect.Type) (number, content []int, ok bool) {
	if isTagType(kind) {
		return []int{0}, []int{1}, true
	}
	type node struct {
		kind  reflect.Type
		index []int
	}
	level := []node{{kind: kind}}
	for range 16 {
		var next []node
		var found []int
		matches := 0
		for _, parent := range level {
			for position := range parent.kind.NumField() {
				field := parent.kind.Field(position)
				target := field.Type
				if target.Kind() == reflect.Pointer {
					target = target.Elem()
				}
				if !field.Anonymous || !field.IsExported() || target.Kind() != reflect.Struct {
					continue
				}
				index := append(slices.Clip(parent.index), position)
				if isTagType(target) {
					matches++
					found = index
					continue
				}
				next = append(next, node{kind: target, index: index})
			}
		}
		switch {
		case matches == 1:
			return append(slices.Clip(found), 0), append(slices.Clip(found), 1), true
		case matches > 1 || len(next) == 0:
			return nil, nil, false
		}
		level = next
	}
	return nil, nil, false
}
