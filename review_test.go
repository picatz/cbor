package cbor_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/picatz/cbor"
)

// TestEncodedSizeLimitIncludesTrailingHeaders checks that headers and
// scalars appended after the last size check still count against the
// 1 MiB item limit.
func TestEncodedSizeLimitIncludesTrailingHeaders(t *testing.T) {
	text := strings.Repeat("a", 1<<20-10)
	for name, value := range map[string]any{
		"array": []any{text, uint64(math.MaxUint64)},
		"struct": struct {
			A string
			B uint64
		}{text, math.MaxUint64},
		"map": map[string]uint64{text: math.MaxUint64},
	} {
		if encoded, err := cbor.Marshal(value); !errors.Is(err, cbor.ErrLimit) {
			t.Errorf("%s: Marshal = %d bytes, %v; want ErrLimit", name, len(encoded), err)
		}
		if _, err := cbor.AppendMarshal(make([]byte, 5), value); !errors.Is(err, cbor.ErrLimit) {
			t.Errorf("%s: AppendMarshal error = %v; want ErrLimit", name, err)
		}
	}
}

type embedsTagPointer struct{ *cbor.Tag[int] }

type holdsTagPointer struct {
	A int
	B embedsTagPointer
}

// TestEmbeddedTagPointer checks that a nil embedded *Tag neither panics nor
// leaves a half-built codec in the type cache.
func TestEmbeddedTagPointer(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 2 {
			var value embedsTagPointer
			if err := cbor.Unmarshal([]byte{0xc1, 0x05}, &value); err != nil || value.Tag == nil || value.Number != 1 || value.Content != 5 {
				t.Errorf("Unmarshal = %+v, %v", value.Tag, err)
			}
			var holder holdsTagPointer
			if err := cbor.Unmarshal([]byte{0xa1, 0x61, 'A', 0x01}, &holder); err != nil || holder.A != 1 {
				t.Errorf("Unmarshal holder = %+v, %v", holder, err)
			}
			if encoded, err := cbor.Marshal(embedsTagPointer{}); err != nil || len(encoded) != 1 || encoded[0] != 0xf6 {
				t.Errorf("Marshal nil embedded tag = %x, %v", encoded, err)
			}
			if encoded, err := cbor.Marshal(embedsTagPointer{&cbor.Tag[int]{Number: 1, Content: 5}}); err != nil || string(encoded) != "\xc1\x05" {
				t.Errorf("Marshal embedded tag = %x, %v", encoded, err)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("decoding hung")
	}
}

type isZeroer interface{ IsZero() bool }

func TestOmitZeroNilInterface(t *testing.T) {
	value := struct {
		Z isZeroer `cbor:"z,omitzero"`
	}{}
	if encoded, err := cbor.Marshal(value); err != nil || string(encoded) != "\xa0" {
		t.Fatalf("Marshal = %x, %v", encoded, err)
	}
}

func TestBigNegativeInterfaceKeys(t *testing.T) {
	data := []byte{0xa1, 0x3b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x01}
	if err := cbor.Unmarshal(data, new(map[any]any)); !errors.Is(err, cbor.ErrType) {
		t.Errorf("map[any]any: %v", err)
	}
	if err := cbor.Unmarshal(data, new(map[any]int)); !errors.Is(err, cbor.ErrType) {
		t.Errorf("map[any]int: %v", err)
	}
}

func TestNullIntoSimpleValue(t *testing.T) {
	var simple cbor.SimpleValue
	if err := cbor.Unmarshal([]byte{0xf6}, &simple); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("null into SimpleValue: %v (%d)", err, simple)
	}
	if err := cbor.Unmarshal([]byte{0xf7}, &simple); err != nil || simple != cbor.SimpleValueUndefined {
		t.Fatalf("undefined into SimpleValue: %v (%d)", err, simple)
	}
}

type ConflictLeft struct{ conflictMiddle }
type ConflictRight struct{ conflictMiddle }
type conflictMiddle struct{ conflictLeaf }
type conflictLeaf struct{ Z int }

// TestDeepSameDepthConflict checks that a key reached twice at the same
// depth through repeated embedding is an error, however deep it is.
func TestDeepSameDepthConflict(t *testing.T) {
	value := struct {
		ConflictLeft
		ConflictRight
	}{}
	if encoded, err := cbor.Marshal(value); !errors.Is(err, cbor.ErrUnsupported) {
		t.Fatalf("Marshal = %x, %v; want ErrUnsupported", encoded, err)
	}
}
