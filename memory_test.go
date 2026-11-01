package cbor_test

import (
	"errors"
	"math"
	"runtime"
	"testing"

	"github.com/picatz/cbor"
)

// TestDecodeMemoryAmplification bounds the memory an adversarial item can
// demand: about 1 MiB of one-byte items, the densest possible input.
func TestDecodeMemoryAmplification(t *testing.T) {
	if testing.Short() || runtime.GOARCH == "wasm" {
		t.Skip("allocates tens of megabytes")
	}
	data := []byte{0x99, 0x27, 0x10} // [[0 x 99] x 10000]
	for range 10000 {
		data = append(data, 0x98, 99)
		data = append(data, make([]byte, 99)...)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if err := cbor.Unmarshal(data, new(cbor.RawMessage)); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	// 24 bytes of index per item, at most doubled while the arena grows,
	// plus the RawMessage copy.
	if ratio := float64(after.TotalAlloc-before.TotalAlloc) / float64(len(data)); ratio > 52 {
		t.Fatalf("decoding %d bytes allocated %.1f bytes per input byte", len(data), ratio)
	}
}

func TestMaxEncodedBytesRange(t *testing.T) {
	options := cbor.DecoderOptions{MaxEncodedBytes: math.MaxInt32}
	if err := options.Unmarshal([]byte{0}, new(any)); err != nil {
		t.Fatalf("MaxInt32: %v", err)
	}
	if math.MaxInt > math.MaxInt32 {
		options.MaxEncodedBytes = int(int64(math.MaxInt32) + 1)
		if err := options.Unmarshal([]byte{0}, new(any)); err == nil || errors.Is(err, cbor.ErrLimit) {
			t.Fatalf("over MaxInt32: %v, want an invalid-options error", err)
		}
	}
}
