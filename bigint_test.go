package cbor_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"math/big"
	"strings"
	"testing"

	"github.com/picatz/cbor"
)

func bigFrom(t *testing.T, decimal string) *big.Int {
	t.Helper()
	value, ok := new(big.Int).SetString(decimal, 10)
	if !ok {
		t.Fatalf("invalid decimal %q", decimal)
	}
	return value
}

// TestBigIntPreferredSerialization checks RFC 8949 Section 3.4.3: integers
// that fit major types 0 and 1 use them, and larger ones use tags 2 and 3
// without leading zero bytes.
func TestBigIntPreferredSerialization(t *testing.T) {
	for _, test := range []struct{ decimal, encoded string }{
		{"0", "00"},
		{"23", "17"},
		{"18446744073709551615", "1bffffffffffffffff"},
		{"18446744073709551616", "c249010000000000000000"},
		{"-1", "20"},
		{"-9223372036854775809", "3b8000000000000000"},
		{"-18446744073709551616", "3bffffffffffffffff"},
		{"-18446744073709551617", "c349010000000000000000"},
		{"340282366920938463463374607431768211456", "c25101" + strings.Repeat("00", 16)},
	} {
		value := bigFrom(t, test.decimal)
		for _, encode := range []any{value, *value} {
			encoded, err := cbor.CoreDeterministicEncoding().Marshal(encode)
			if err != nil || hex.EncodeToString(encoded) != test.encoded {
				t.Errorf("Marshal(%s) = %x, %v; want %s", test.decimal, encoded, err, test.encoded)
			}
		}
		strict := cbor.DecoderOptions{RequireDeterministic: true}
		var pointer *big.Int
		if err := strict.Unmarshal(mustHex(test.encoded), &pointer); err != nil || pointer.Cmp(value) != 0 {
			t.Errorf("Unmarshal(%s) into *big.Int = %v, %v; want %s", test.encoded, pointer, err, test.decimal)
		}
		var plain big.Int
		if err := strict.Unmarshal(mustHex(test.encoded), &plain); err != nil || plain.Cmp(value) != 0 {
			t.Errorf("Unmarshal(%s) into big.Int = %v, %v", test.encoded, &plain, err)
		}
	}
}

func TestBigIntDecoding(t *testing.T) {
	// Decoders that understand bignums must accept leading zeros and values
	// that would fit in an integer; the deterministic decoder must not.
	for _, encoded := range []string{"c24100", "c2420001", "c348000000000000000a", "c240"} {
		var value big.Int
		if err := cbor.Unmarshal(mustHex(encoded), &value); err != nil {
			t.Errorf("Unmarshal(%s): %v", encoded, err)
		}
		strict := cbor.DecoderOptions{RequireDeterministic: true}
		if err := strict.Unmarshal(mustHex(encoded), &value); !errors.Is(err, cbor.ErrNotDeterministic) {
			t.Errorf("RequireDeterministic Unmarshal(%s) = %v, want ErrNotDeterministic", encoded, err)
		}
	}
	for _, encoded := range []string{"c401", "c26161", "6161", "f93c00", "c3f6"} {
		var value big.Int
		if err := cbor.Unmarshal(mustHex(encoded), &value); !errors.Is(err, cbor.ErrType) {
			t.Errorf("Unmarshal(%s) = %v, want ErrType", encoded, err)
		}
	}
	var null *big.Int
	if err := cbor.Unmarshal(mustHex("f6"), &null); err != nil || null != nil {
		t.Errorf("null into *big.Int = %v, %v", null, err)
	}
}

// TestNegativeIntegerRange covers the full major type 1 range, which decodes
// into int64 only where it fits.
func TestNegativeIntegerRange(t *testing.T) {
	var small int64
	if err := cbor.Unmarshal(mustHex("3b7fffffffffffffff"), &small); err != nil || small != math.MinInt64 {
		t.Fatalf("MinInt64 = %d, %v", small, err)
	}
	if err := cbor.Unmarshal(mustHex("3b8000000000000000"), &small); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("below MinInt64 into int64: %v", err)
	}
	var unsigned uint64
	if err := cbor.Unmarshal(mustHex("20"), &unsigned); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("negative into uint64: %v", err)
	}
	var float float64
	if err := cbor.Unmarshal(mustHex("3bffffffffffffffff"), &float); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("below MinInt64 into float64: %v", err)
	}
	var generic any
	if err := cbor.Unmarshal(mustHex("3b8000000000000000"), &generic); err != nil {
		t.Fatal(err)
	}
	if value, ok := generic.(*big.Int); !ok || value.String() != "-9223372036854775809" {
		t.Fatalf("generic = %#v", generic)
	}
	encoded, err := cbor.Marshal(generic)
	if err != nil || hex.EncodeToString(encoded) != "3b8000000000000000" {
		t.Fatalf("re-encoded %x, %v", encoded, err)
	}
	// Such keys would compare by pointer in map[any]any, so they need a
	// typed or raw destination.
	if err := cbor.Unmarshal(mustHex("a13bffffffffffffffff00"), &generic); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("big negative key into any: %v", err)
	}
	// Maps keyed by large negative integers keep distinct keys distinct.
	if err := cbor.Unmarshal(mustHex("a23bfffffffffffffffe003bffffffffffffffff01"), new(cbor.RawMessage)); err != nil {
		t.Fatal(err)
	}
	if err := cbor.Unmarshal(mustHex("a23bffffffffffffffff003bffffffffffffffff01"), new(cbor.RawMessage)); !errors.Is(err, cbor.ErrInvalid) {
		t.Fatalf("duplicate large negative keys: %v", err)
	}
}

func TestAppendMarshal(t *testing.T) {
	prefix := []byte{0xd9, 0xd9, 0xf7} // self-described CBOR
	buffer := append(make([]byte, 0, 64), prefix...)
	buffer, err := cbor.AppendMarshal(buffer, map[string]int{"a": 1})
	if err != nil || hex.EncodeToString(buffer) != "d9d9f7a1616101" {
		t.Fatalf("AppendMarshal = %x, %v", buffer, err)
	}
	buffer, err = cbor.CoreDeterministicEncoding().AppendMarshal(buffer, 1.5)
	if err != nil || hex.EncodeToString(buffer) != "d9d9f7a1616101f93e00" {
		t.Fatalf("AppendMarshal = %x, %v", buffer, err)
	}
	// On error the original slice comes back.
	failed, err := cbor.AppendMarshal(buffer, make(chan int))
	if !errors.Is(err, cbor.ErrUnsupported) || !bytes.Equal(failed, buffer) {
		t.Fatalf("AppendMarshal error = %x, %v", failed, err)
	}
	// The item limit applies to the appended item, not the whole buffer.
	large := make([]byte, 2<<20)
	if _, err := cbor.AppendMarshal(large, 1); err != nil {
		t.Fatalf("AppendMarshal after a large prefix: %v", err)
	}
	value := &struct {
		Algorithm int    `cbor:"1,keyasint"`
		KeyID     []byte `cbor:"4,keyasint"`
	}{Algorithm: -7, KeyID: []byte("11")}
	allocations := testing.AllocsPerRun(100, func() {
		buffer, _ = cbor.AppendMarshal(buffer[:0], value)
	})
	if allocations != 0 {
		t.Errorf("AppendMarshal allocates %v times per run", allocations)
	}
}

func TestDecoderAll(t *testing.T) {
	stream := bytes.NewReader(mustHex("01" + "6161" + "820203" + "a0"))
	var items []string
	for message, err := range cbor.NewDecoder(stream).All() {
		if err != nil {
			t.Fatal(err)
		}
		notation, err := cbor.Diagnose(message)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, notation)
	}
	if got := strings.Join(items, ", "); got != `1, "a", [2, 3], {}` {
		t.Fatalf("items = %s", got)
	}

	// A truncated final item is reported once, after the complete ones.
	var errs []error
	count := 0
	for _, err := range cbor.NewDecoder(bytes.NewReader(mustHex("0182"))).All() {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		count++
	}
	if count != 1 || len(errs) != 1 || !errors.Is(errs[0], io.ErrUnexpectedEOF) {
		t.Fatalf("count %d, errors %v", count, errs)
	}

	// Breaking early stops reading.
	reader := bytes.NewReader(mustHex("010203"))
	for range cbor.NewDecoder(reader).All() {
		break
	}
	if reader.Len() != 2 {
		t.Fatalf("read past the first item: %d bytes left", reader.Len())
	}

	// Each message owns its bytes.
	var messages []cbor.RawMessage
	for message, err := range cbor.NewDecoder(bytes.NewReader(mustHex("0102"))).All() {
		if err != nil {
			t.Fatal(err)
		}
		messages = append(messages, message)
	}
	if len(messages) != 2 || messages[0][0] != 1 || messages[1][0] != 2 {
		t.Fatalf("messages = %x", messages)
	}
}
