package cbor

import (
	"testing"
	"unsafe"
)

// The arena's memory bound in DecoderOptions.MaxEncodedBytes assumes this
// size; reordering item's fields can silently add padding.
func TestItemSize(t *testing.T) {
	if size := unsafe.Sizeof(item{}); size != 24 {
		t.Fatalf("item is %d bytes, want 24", size)
	}
}
