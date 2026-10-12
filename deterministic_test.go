package cbor_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"reflect"
	"testing"
	"testing/iotest"

	"github.com/picatz/cbor"
)

func TestEncoderSortModes(t *testing.T) {
	// Keys whose orders differ between the three modes: the integer 24 takes
	// two bytes, the empty text string one, and "a" two.
	value := map[any]bool{uint64(24): true, "": true, "a": true, int64(-1): true}
	for _, test := range []struct {
		mode cbor.SortMode
		want string
	}{
		// Length first: 20, 60 (one byte), then 1818, 6161 (two bytes).
		{cbor.SortLengthFirst, "a420f560f51818f56161f5"},
		// Bytewise: 1818 < 20 < 60 < 6161.
		{cbor.SortBytewiseLexical, "a41818f520f560f56161f5"},
	} {
		encoded, err := cbor.EncoderOptions{Sort: test.mode}.Marshal(value)
		if err != nil || hex.EncodeToString(encoded) != test.want {
			t.Errorf("mode %d: %x, %v; want %s", test.mode, encoded, err, test.want)
		}
	}
	if _, err := (cbor.EncoderOptions{Sort: 9}).Marshal(1); err == nil {
		t.Error("accepted unknown sort mode")
	}
	if _, err := (cbor.EncoderOptions{Floats: 9}).Marshal(1); err == nil {
		t.Error("accepted unknown float mode")
	}
}

func TestShortestFloatEncoding(t *testing.T) {
	shortest := cbor.EncoderOptions{Floats: cbor.FloatShortest}
	for _, test := range []struct {
		value float64
		want  string
	}{
		{0, "f90000"}, {math.Copysign(0, -1), "f98000"}, {1, "f93c00"}, {1.5, "f93e00"},
		{65504, "f97bff"}, {65505, "fa477fe100"}, {100000, "fa47c35000"},
		{5.960464477539063e-8, "f90001"}, {0x1p-25, "fa33000000"}, {6.103515625e-05, "f90400"},
		{0x1.ff8p-15, "f903ff"}, {0x1.ffcp-15, "fa387fe000"}, {1.1, "fb3ff199999999999a"}, {math.MaxFloat32, "fa7f7fffff"},
		{math.Inf(1), "f97c00"}, {math.Inf(-1), "f9fc00"}, {math.NaN(), "f97e00"},
		{math.Float64frombits(0x7ff0000000000001), "f97e00"}, {math.SmallestNonzeroFloat64, "fb0000000000000001"},
	} {
		encoded, err := shortest.Marshal(test.value)
		if err != nil || hex.EncodeToString(encoded) != test.want {
			t.Errorf("%v: %x, %v; want %s", test.value, encoded, err, test.want)
		}
	}
	if encoded, err := shortest.Marshal(float32(0.1)); err != nil || hex.EncodeToString(encoded) != "fa3dcccccd" {
		t.Errorf("float32(0.1) = %x, %v", encoded, err)
	}
	if encoded, err := cbor.Marshal(1.5); err != nil || hex.EncodeToString(encoded) != "fb3ff8000000000000" {
		t.Errorf("default float = %x, %v", encoded, err)
	}
}

func TestRequireDeterministic(t *testing.T) {
	strict := cbor.DecoderOptions{RequireDeterministic: true}
	for _, test := range []struct {
		encoded string
		err     error
	}{
		{"a201020304", nil},
		{"a203040102", cbor.ErrNotDeterministic},     // keys out of order
		{"a2616101381802", cbor.ErrNotDeterministic}, // length-first, not bytewise
		{"a201020102", cbor.ErrInvalid},              // duplicates are invalid, not merely unordered
		{"1801", cbor.ErrNotDeterministic},           // non-minimal argument
		{"5801ff", cbor.ErrNotDeterministic},         // non-minimal length
		{"d80101", cbor.ErrNotDeterministic},         // non-minimal tag number
		{"fa3fc00000", cbor.ErrNotDeterministic},     // 1.5 fits binary16
		{"fb3ff8000000000000", cbor.ErrNotDeterministic},
		{"f97e01", cbor.ErrNotDeterministic}, // NaN with a payload
		{"f9fe00", cbor.ErrNotDeterministic}, // negative quiet NaN
		{"fa477fe100", nil},
		{"0001", cbor.ErrMalformed}, // trailing data stays malformed
	} {
		data, _ := hex.DecodeString(test.encoded)
		var value any
		err := strict.Unmarshal(data, &value)
		if test.err == nil && err != nil || test.err != nil && !errors.Is(err, test.err) {
			t.Errorf("%s: %v, want %v", test.encoded, err, test.err)
		}
	}
	// Map keys are compared within each map, not across nested maps.
	nested := mustMarshalWith(t, cbor.CoreDeterministicEncoding(), map[string]any{"b": map[string]int{"z": 1, "a": 2}, "a": 1})
	var value any
	if err := strict.Unmarshal(nested, &value); err != nil {
		t.Fatalf("%x: %v", nested, err)
	}
	// The stream decoder enforces the same contract.
	data, _ := hex.DecodeString("a203040102")
	if err := cbor.NewDecoderWithOptions(iotest.OneByteReader(bytes.NewReader(data)), strict).Decode(&value); !errors.Is(err, cbor.ErrNotDeterministic) {
		t.Fatal(err)
	}
}

func TestDeterministicEncoderRejectsNonDeterministicRaw(t *testing.T) {
	deterministic := cbor.CoreDeterministicEncoding()
	for _, raw := range []string{"1801", "a203040102", "fb3ff8000000000000"} {
		data, _ := hex.DecodeString(raw)
		if _, err := deterministic.Marshal([]any{cbor.RawMessage(data)}); !errors.Is(err, cbor.ErrNotDeterministic) {
			t.Errorf("%s: %v", raw, err)
		}
		if _, err := cbor.Marshal(cbor.RawMessage(data)); err != nil {
			t.Errorf("default encoder rejected valid raw %s: %v", raw, err)
		}
	}
}

func TestErrorKindsAndOffsets(t *testing.T) {
	for _, test := range []struct {
		encoded string
		kind    error
		offset  int64
	}{
		{"1c", cbor.ErrMalformed, 0},
		{"8201ff", cbor.ErrMalformed, 2},
		{"0102", cbor.ErrMalformed, 1},
		{"f818", cbor.ErrMalformed, 0},
		{"9f01ff", cbor.ErrUnsupported, 0},
		{"a1820102f5", cbor.ErrUnsupported, 1},
		{"a1f97e00f5", cbor.ErrUnsupported, 1},
		{"3bffffffffffffffff", cbor.ErrUnsupported, 0},
		{"a301020304010f", cbor.ErrInvalid, 5},
		{"82616162c328", cbor.ErrInvalid, 4},
	} {
		data, _ := hex.DecodeString(test.encoded)
		var value any
		err := cbor.Unmarshal(data, &value)
		var detail *cbor.Error
		if !errors.Is(err, test.kind) || !errors.As(err, &detail) || detail.Offset != test.offset {
			t.Errorf("%s: %v (offset %v), want %v at %d", test.encoded, err, detail, test.kind, test.offset)
		}
	}
	var number uint8
	if err := cbor.Unmarshal([]byte{0x19, 1, 0}, &number); !errors.Is(err, cbor.ErrType) {
		t.Errorf("overflow: %v", err)
	}
	if err := (cbor.DecoderOptions{MaxBytes: 1}).Unmarshal([]byte{0x42, 0, 0}, new([]byte)); !errors.Is(err, cbor.ErrLimit) {
		t.Errorf("limit: %v", err)
	}
	if _, err := cbor.Marshal(make(chan int)); !errors.Is(err, cbor.ErrUnsupported) {
		t.Errorf("chan: %v", err)
	}
	if _, err := cbor.Marshal("\xff"); !errors.Is(err, cbor.ErrInvalid) {
		t.Errorf("UTF-8: %v", err)
	}
	if err := cbor.Unmarshal(nil, new(any)); err != io.EOF {
		t.Errorf("empty input: %v", err)
	}
	if want := "cbor: duplicate map key at offset 5"; func() string {
		err := cbor.Unmarshal([]byte{0xa3, 1, 2, 3, 4, 1, 15}, new(any))
		return err.Error()
	}() != want {
		t.Errorf("error text changed; want %q", want)
	}
}

func TestSimpleValues(t *testing.T) {
	for _, test := range []struct {
		value cbor.SimpleValue
		want  string
	}{
		{0, "e0"}, {16, "f0"}, {cbor.SimpleValueFalse, "f4"}, {cbor.SimpleValueUndefined, "f7"}, {32, "f820"}, {255, "f8ff"},
	} {
		encoded, err := cbor.Marshal(test.value)
		if err != nil || hex.EncodeToString(encoded) != test.want {
			t.Errorf("%d: %x, %v", test.value, encoded, err)
		}
		var decoded cbor.SimpleValue
		if err := cbor.Unmarshal(encoded, &decoded); err != nil || decoded != test.value {
			t.Errorf("%s: %d, %v", test.want, decoded, err)
		}
	}
	for _, invalid := range []cbor.SimpleValue{24, 31, 256, -1} {
		if _, err := cbor.Marshal(invalid); !errors.Is(err, cbor.ErrInvalid) {
			t.Errorf("%d: %v", invalid, err)
		}
	}
	var decoded cbor.SimpleValue
	if err := cbor.Unmarshal([]byte{0xf9, 0, 0}, &decoded); !errors.Is(err, cbor.ErrType) {
		t.Errorf("float into SimpleValue: %v", err)
	}
}

func TestByteArrays(t *testing.T) {
	var aaguid [16]byte
	encoded := append([]byte{0x50}, bytes.Repeat([]byte{7}, 16)...)
	if err := cbor.Unmarshal(encoded, &aaguid); err != nil || aaguid[15] != 7 {
		t.Fatal(aaguid, err)
	}
	if err := cbor.Unmarshal(encoded[:9:9], new([16]byte)); err == nil {
		t.Fatal("accepted truncated input")
	}
	if encoded, err := cbor.Marshal(aaguid); err != nil || !bytes.Equal(encoded, append([]byte{0x50}, bytes.Repeat([]byte{7}, 16)...)) {
		t.Fatalf("[16]byte encoded as %x, %v", encoded, err)
	}
	type wrapper struct{ ID [2]byte }
	if encoded, err := cbor.Marshal(wrapper{ID: [2]byte{1, 2}}); err != nil || hex.EncodeToString(encoded) != "a1624944420102" {
		t.Fatalf("struct field [2]byte encoded as %x, %v", encoded, err)
	}
	short := append([]byte{0x4f}, bytes.Repeat([]byte{7}, 15)...)
	if err := cbor.Unmarshal(short, &aaguid); !errors.Is(err, cbor.ErrType) {
		t.Fatal("accepted wrong-length byte string", err)
	}
}

func mustMarshalWith(t *testing.T, options cbor.EncoderOptions, value any) []byte {
	t.Helper()
	encoded, err := options.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// ctap2Less is the CTAP2 canonical CBOR key order, written independently of
// the encoder: major type, then encoded length, then bytewise.
func ctap2Less(left, right []byte) bool {
	if left[0]>>5 != right[0]>>5 {
		return left[0]>>5 < right[0]>>5
	}
	if len(left) != len(right) {
		return len(left) < len(right)
	}
	return bytes.Compare(left, right) < 0
}

// FuzzBytewiseMatchesCTAP2 checks the claim in the SortBytewiseLexical
// documentation: for preferred-serialized keys, CTAP2 order is bytewise order.
func FuzzBytewiseMatchesCTAP2(f *testing.F) {
	f.Add(uint64(24), "", int64(-1), true)
	f.Add(uint64(1000), "abcdefghijklmnopqrstuvwxyz", int64(-1000), false)
	f.Fuzz(func(t *testing.T, number uint64, text string, negative int64, flag bool) {
		if negative >= 0 {
			negative = -1 - negative
		}
		var keys [][]byte
		for _, key := range []any{number, text, negative, flag, []byte(text), number / 3, text + text} {
			encoded, err := cbor.Marshal(key)
			if err != nil {
				return // invalid UTF-8 text
			}
			keys = append(keys, encoded)
		}
		for _, left := range keys {
			for _, right := range keys {
				if ctap2Less(left, right) != (bytes.Compare(left, right) < 0) {
					t.Fatalf("orders differ for %x and %x", left, right)
				}
			}
		}
	})
}

// TestDecodedValuesDoNotAliasInput guards the ownership contract: Unmarshal
// reads directly from the caller's slice, so every decoded byte slice must be
// a copy.
func TestDecodedValuesDoNotAliasInput(t *testing.T) {
	type record struct {
		Bytes  []byte            `cbor:"1,keyasint"`
		Array  [2]byte           `cbor:"2,keyasint"`
		Raw    cbor.RawMessage   `cbor:"3,keyasint"`
		Any    any               `cbor:"4,keyasint"`
		Text   string            `cbor:"5,keyasint"`
		Nested map[string][]byte `cbor:"6,keyasint"`
	}
	original := record{
		Bytes: []byte{1, 2}, Array: [2]byte{3, 4}, Raw: cbor.RawMessage{0x42, 5, 6},
		Any: cbor.TaggedValue{Number: 24, Value: []byte{7, 8}}, Text: "ab", Nested: map[string][]byte{"k": {9, 10}},
	}
	encoded, err := cbor.Marshal(original)
	if err != nil {
		t.Fatal(err)
	}
	var generic any
	var decoded record
	if err := cbor.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := cbor.Unmarshal(encoded, &generic); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := cbor.Marshal(generic)
	for index := range encoded {
		encoded[index] = 0xff
	}
	original.Any = cbor.TaggedValue{Number: 24, Value: []byte{7, 8}}
	if !reflect.DeepEqual(decoded, original) {
		t.Fatalf("decoded value changed with input: %#v", decoded)
	}
	if again, _ := cbor.Marshal(generic); !bytes.Equal(again, snapshot) {
		t.Fatal("generic value changed with input")
	}
}

// TestNestingSymmetry checks that anything decoded within the default nesting
// limit re-encodes: interface values must not count as extra levels.
func TestNestingSymmetry(t *testing.T) {
	nested := func(levels int) []byte {
		data := bytes.Repeat([]byte{0x81}, levels)
		for index := 1; index < levels; index += 3 {
			data[index] = 0xc6 // tag 6 also adds a level
		}
		return append(data, 0)
	}
	var value any
	if err := cbor.Unmarshal(nested(64), &value); err != nil {
		t.Fatal(err)
	}
	if _, err := cbor.Marshal(value); err != nil {
		t.Fatalf("re-encoding 64 levels: %v", err)
	}
	if err := cbor.Unmarshal(nested(65), &value); !errors.Is(err, cbor.ErrLimit) {
		t.Fatalf("65 levels: %v", err)
	}
}
