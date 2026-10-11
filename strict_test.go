package cbor_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"math"
	"reflect"
	"strings"
	"sync"
	"testing"
	"testing/iotest"

	"github.com/picatz/cbor"
)

func TestStrictMalformedInput(t *testing.T) {
	for _, encoded := range []string{
		"63ff", "61ff", "1c", "1f", "9f01ff", "a201020103", "a20102180103",
		"a2616101616102", "a1", "82f5", "1bffffffffffffffff00", "3bffffffffffffffff",
		"a1810000", "a1f97e0000", "a201020100", "f818",
	} {
		t.Run(encoded, func(t *testing.T) {
			data, err := hex.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			var raw cbor.RawMessage
			if err := cbor.Unmarshal(data, &raw); err == nil {
				t.Fatalf("accepted malformed item %x", data)
			}
		})
	}
}

func TestDecodeRawAndUnknownFields(t *testing.T) {
	data, _ := hex.DecodeString("a30182684649444f5f325f30684649444f5f325f3104a262726bf569636c69656e7450696ef418638201a16178f6")
	var fields map[uint64]cbor.RawMessage
	if err := cbor.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	var versions []string
	if err := cbor.Unmarshal(fields[1], &versions); err != nil || !reflect.DeepEqual(versions, []string{"FIDO_2_0", "FIDO_2_1"}) {
		t.Fatal(versions, err)
	}
	var options map[string]bool
	if err := cbor.Unmarshal(fields[4], &options); err != nil || !options["rk"] || options["clientPin"] {
		t.Fatal(options, err)
	}
	if got := hex.EncodeToString(fields[99]); got != "8201a16178f6" {
		t.Fatal(got)
	}
	copy(data, bytes.Repeat([]byte{0}, len(data)))
	if fields[1][0] != 0x82 {
		t.Fatal("RawMessage aliases input")
	}
	var selected struct {
		Options map[string]bool `cbor:"4,keyasint"`
	}
	encoded, err := cbor.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if err := cbor.Unmarshal(encoded, &selected); err != nil || !selected.Options["rk"] {
		t.Fatal(selected, err)
	}
}

func TestDecodeBoundariesAndAtomicity(t *testing.T) {
	for _, data := range [][]byte{{0xa2, 0x61, 'a', 1}, {0xa1, 0x61, 'a', 0xf5}, {0xa1, 0x61, 'a', 2, 0}} {
		value := map[string]int{"original": 7}
		if err := cbor.Unmarshal(data, &value); err == nil {
			t.Fatalf("accepted %x", data)
		}
		if !reflect.DeepEqual(value, map[string]int{"original": 7}) {
			t.Fatal("failed decode changed destination", value)
		}
	}
	var value uint8 = 7
	if err := cbor.Unmarshal([]byte{0x19, 1, 0}, &value); err == nil || value != 7 {
		t.Fatal(value, err)
	}
	for _, destination := range []any{nil, 1, (*int)(nil)} {
		if err := cbor.Unmarshal([]byte{1}, destination); err == nil {
			t.Fatal("accepted invalid destination")
		}
	}
	var words []string = []string{"old"}
	if err := cbor.Unmarshal([]byte{0x82, 0x61, 'a', 0x61, 'b'}, &words); err != nil || len(words) != 2 {
		t.Fatal(words, err)
	}
}

func TestDecoderStreamAndShortReads(t *testing.T) {
	data := []byte{0x1a, 1, 2, 3, 4, 0x63, 'a', 'b', 'c', 0xf5}
	decoder := cbor.NewDecoder(iotest.OneByteReader(bytes.NewReader(data)))
	var number uint64
	var text string
	var flag bool
	if err := decoder.Decode(&number); err != nil || number != 0x01020304 {
		t.Fatal(number, err)
	}
	if err := decoder.Decode(&text); err != nil || text != "abc" {
		t.Fatal(text, err)
	}
	if err := decoder.Decode(&flag); err != nil || !flag {
		t.Fatal(flag, err)
	}
	if err := decoder.Decode(&flag); !errors.Is(err, io.EOF) {
		t.Fatal(err)
	}
	for _, truncated := range [][]byte{{0x19}, {0x19, 1}, {0x63, 'a'}, {0x81}, {0xa1, 1}} {
		var raw cbor.RawMessage
		if err := cbor.NewDecoder(bytes.NewReader(truncated)).Decode(&raw); !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("%x: %v", truncated, err)
		}
	}
}

func TestDecoderLimitsAndIsolation(t *testing.T) {
	for _, test := range []struct {
		options cbor.DecoderOptions
		data    []byte
	}{
		{cbor.DecoderOptions{MaxArrayElements: 1}, []byte{0x82, 0, 0}},
		{cbor.DecoderOptions{MaxMapPairs: 1}, []byte{0xa2, 1, 0, 2, 0}},
		{cbor.DecoderOptions{MaxStringBytes: 1}, []byte{0x62, 'a', 'b'}},
		{cbor.DecoderOptions{MaxBytes: 1}, []byte{0x42, 0, 0}},
		{cbor.DecoderOptions{MaxEncodedBytes: 2}, []byte{0x82, 0, 0}},
		{cbor.DecoderOptions{MaxNestedLevels: 1}, []byte{0x81, 0x81, 0}},
		{cbor.DecoderOptions{RejectTags: true}, []byte{0xc0, 0}},
		{cbor.DecoderOptions{RejectNonMinimal: true}, []byte{0x18, 1}},
		{cbor.DecoderOptions{MaxBytes: -1}, []byte{0}},
	} {
		var raw cbor.RawMessage
		if err := test.options.Unmarshal(test.data, &raw); err == nil {
			t.Fatalf("limit ignored: %+v %x", test.options, test.data)
		}
	}
	first := cbor.NewDecoder(bytes.NewReader([]byte{0x82, 0, 0}))
	second := cbor.NewDecoder(bytes.NewReader([]byte{0x82, 0, 0}))
	first.SetMaxArrayElements(1)
	var values []uint64
	if first.Decode(&values) == nil {
		t.Fatal("first limit ignored")
	}
	if err := second.Decode(&values); err != nil {
		t.Fatal("options shared between decoders", err)
	}
}

func TestIndependentStructDecodes(t *testing.T) {
	type record struct {
		Value   int    `cbor:"value"`
		Ignored string `cbor:"-"`
		hidden  int
	}
	var first, second record
	if err := cbor.Unmarshal([]byte{0xa1, 0x65, 'v', 'a', 'l', 'u', 'e', 1}, &first); err != nil {
		t.Fatal(err)
	}
	if err := cbor.Unmarshal([]byte{0xa1, 0x65, 'v', 'a', 'l', 'u', 'e', 2}, &second); err != nil {
		t.Fatal(err)
	}
	if first.Value != 1 || second.Value != 2 {
		t.Fatal(first, second)
	}
	var workers sync.WaitGroup
	for worker := 0; worker < 20; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			var result record
			if err := cbor.Unmarshal([]byte{0xa1, 0x65, 'v', 'a', 'l', 'u', 'e', 3}, &result); err != nil || result.Value != 3 {
				t.Errorf("concurrent decode: %+v %v", result, err)
			}
		}()
	}
	workers.Wait()
}

func TestEncoderVectors(t *testing.T) {
	for _, test := range []struct {
		value   any
		encoded string
	}{
		{int64(-1), "20"}, {int64(-25), "3818"}, {int64(math.MinInt64), "3b7fffffffffffffff"},
		{int64(math.MaxInt64), "1b7fffffffffffffff"}, {uint64(math.MaxUint64), "1bffffffffffffffff"},
		{[]byte{1, 2}, "420102"}, {"hello", "6568656c6c6f"}, {[]int{1, 2}, "820102"},
		{map[string]bool{"rk": true}, "a162726bf5"}, {cbor.RawMessage{0x81, 1}, "8101"}, {cbor.RawMessage(nil), "f6"},
		{struct {
			Value  int    `cbor:"1,keyasint"`
			Hidden string `cbor:"-"`
		}{Value: -7}, "a10126"},
	} {
		encoded, err := cbor.Marshal(test.value)
		if err != nil || hex.EncodeToString(encoded) != test.encoded {
			t.Fatalf("Marshal(%v) = %x, %v; want %s", test.value, encoded, err, test.encoded)
		}
	}
	for _, value := range []any{strings.Repeat("a", 256), make([]uint64, 256), map[string]any{"options": map[string]bool{"rk": true}}} {
		encoded, err := cbor.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded any
		if err := cbor.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := cbor.Marshal(cbor.RawMessage{1, 2}); err == nil {
		t.Fatal("accepted multiple raw items")
	}
	var cycle any
	cycle = &cycle
	if _, err := cbor.Marshal(cycle); err == nil {
		t.Fatal("accepted cycle")
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) { return len(data) - 1, nil }

func TestEncoderShortWrite(t *testing.T) {
	if err := cbor.NewEncoder(shortWriter{}).Encode("hello"); !errors.Is(err, io.ErrShortWrite) {
		t.Fatal(err)
	}
}

func TestFloatingPointNullBitPattern(t *testing.T) {
	data := []byte{0xfb, 0, 0, 0, 0, 0, 0, 0, 22}
	var value *float64
	if err := cbor.Unmarshal(data, &value); err != nil || value == nil || math.Float64bits(*value) != 22 {
		t.Fatal(value, err)
	}
}

func TestEncoderRawNestingBudget(t *testing.T) {
	raw := cbor.RawMessage(append(bytes.Repeat([]byte{0x81}, 64), 0))
	if _, err := cbor.Marshal([]any{raw}); err == nil {
		t.Fatal("raw item bypassed aggregate nesting limit")
	}
}

func FuzzDecode(fuzzer *testing.F) {
	for _, seed := range [][]byte{{0xa1, 0x62, 'r', 'k', 0xf5}, {0x82, 1, 2}, {0xf6}, {0x63, 'a'}, {0xa1, 0x81, 0, 1}, {0x1b, 255, 255, 255, 255, 255, 255, 255, 255}} {
		fuzzer.Add(seed)
	}
	fuzzer.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 8192 {
			t.Skip()
		}
		options := cbor.DecoderOptions{MaxEncodedBytes: 8192, MaxNestedLevels: 16, MaxArrayElements: 256, MaxMapPairs: 128, MaxBytes: 8192, MaxStringBytes: 8192}
		for _, destination := range []any{new(any), new(map[uint64]cbor.RawMessage), new(map[string]bool), new([]string), new(int8), new(float32), new(struct{ Value bool })} {
			_ = options.Unmarshal(data, destination)
		}
		var raw cbor.RawMessage
		if options.Unmarshal(data, &raw) == nil && !bytes.Equal(raw, data) {
			t.Fatal("raw round trip changed bytes")
		}
	})
}
