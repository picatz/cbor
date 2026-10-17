package cbor

import "reflect"

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
type tagEncoder interface{ cborTag() (uint64, reflect.Value) }

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
