package cbor_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math"
	"reflect"
	"testing"

	"github.com/picatz/cbor"
)

// appendixA lists every example from RFC 8949 Appendix A, in order, with the
// result this package's profile defines for it. The vectors were cross-checked
// against the machine-readable copy at https://github.com/cbor/test-vectors
// (appendix_a.json), which also carries "f818": RFC 7049 listed it, but RFC
// 8949 Section 3.3 makes two-byte simple values below 32 not well-formed.
//
// deterministic reports whether the example is already in core deterministic
// encoding, which corresponds to that file's "roundtrip" flag.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#appendix-A
var appendixA = []struct {
	encoded       string
	want          any
	err           error
	deterministic bool
}{
	{"00", uint64(0), nil, true},
	{"01", uint64(1), nil, true},
	{"0a", uint64(10), nil, true},
	{"17", uint64(23), nil, true},
	{"1818", uint64(24), nil, true},
	{"1819", uint64(25), nil, true},
	{"1864", uint64(100), nil, true},
	{"1903e8", uint64(1000), nil, true},
	{"1a000f4240", uint64(1000000), nil, true},
	{"1b000000e8d4a51000", uint64(1000000000000), nil, true},
	{"1bffffffffffffffff", uint64(18446744073709551615), nil, true},
	{"c249010000000000000000", cbor.TaggedValue{Number: 2, Value: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0}}, nil, true},
	{"3bffffffffffffffff", nil, cbor.ErrUnsupported, true},
	{"c349010000000000000000", cbor.TaggedValue{Number: 3, Value: []byte{1, 0, 0, 0, 0, 0, 0, 0, 0}}, nil, true},
	{"20", int64(-1), nil, true},
	{"29", int64(-10), nil, true},
	{"3863", int64(-100), nil, true},
	{"3903e7", int64(-1000), nil, true},
	{"f90000", 0.0, nil, true},
	{"f98000", math.Copysign(0, -1), nil, true},
	{"f93c00", 1.0, nil, true},
	{"fb3ff199999999999a", 1.1, nil, true},
	{"f93e00", 1.5, nil, true},
	{"f97bff", 65504.0, nil, true},
	{"fa47c35000", 100000.0, nil, true},
	{"fa7f7fffff", 3.4028234663852886e+38, nil, true},
	{"fb7e37e43c8800759c", 1.0e+300, nil, true},
	{"f90001", 5.960464477539063e-8, nil, true},
	{"f90400", 0.00006103515625, nil, true},
	{"f9c400", -4.0, nil, true},
	{"fbc010666666666666", -4.1, nil, true},
	{"f97c00", math.Inf(1), nil, true},
	{"f97e00", math.NaN(), nil, true},
	{"f9fc00", math.Inf(-1), nil, true},
	{"fa7f800000", math.Inf(1), nil, false},
	{"fa7fc00000", math.NaN(), nil, false},
	{"faff800000", math.Inf(-1), nil, false},
	{"fb7ff0000000000000", math.Inf(1), nil, false},
	{"fb7ff8000000000000", math.NaN(), nil, false},
	{"fbfff0000000000000", math.Inf(-1), nil, false},
	{"f4", false, nil, true},
	{"f5", true, nil, true},
	{"f6", nil, nil, true},
	{"f7", cbor.SimpleValueUndefined, nil, true},
	{"f0", cbor.SimpleValue(16), nil, true},
	{"f818", nil, cbor.ErrMalformed, false},
	{"f8ff", cbor.SimpleValue(255), nil, true},
	{"c074323031332d30332d32315432303a30343a30305a", cbor.TaggedValue{Number: 0, Value: "2013-03-21T20:04:00Z"}, nil, true},
	{"c11a514b67b0", cbor.TaggedValue{Number: 1, Value: uint64(1363896240)}, nil, true},
	{"c1fb41d452d9ec200000", cbor.TaggedValue{Number: 1, Value: 1363896240.5}, nil, true},
	{"d74401020304", cbor.TaggedValue{Number: 23, Value: []byte{1, 2, 3, 4}}, nil, true},
	{"d818456449455446", cbor.TaggedValue{Number: 24, Value: []byte("dIETF")}, nil, true},
	{"d82076687474703a2f2f7777772e6578616d706c652e636f6d", cbor.TaggedValue{Number: 32, Value: "http://www.example.com"}, nil, true},
	{"40", []byte{}, nil, true},
	{"4401020304", []byte{1, 2, 3, 4}, nil, true},
	{"60", "", nil, true},
	{"6161", "a", nil, true},
	{"6449455446", "IETF", nil, true},
	{"62225c", "\"\\", nil, true},
	{"62c3bc", "ü", nil, true},
	{"63e6b0b4", "水", nil, true},
	{"64f0908591", "\U00010151", nil, true},
	{"80", []any{}, nil, true},
	{"83010203", []any{uint64(1), uint64(2), uint64(3)}, nil, true},
	{"8301820203820405", []any{uint64(1), []any{uint64(2), uint64(3)}, []any{uint64(4), uint64(5)}}, nil, true},
	{"98190102030405060708090a0b0c0d0e0f101112131415161718181819", oneThrough25(), nil, true},
	{"a0", map[any]any{}, nil, true},
	{"a201020304", map[any]any{uint64(1): uint64(2), uint64(3): uint64(4)}, nil, true},
	{"a26161016162820203", map[any]any{"a": uint64(1), "b": []any{uint64(2), uint64(3)}}, nil, true},
	{"826161a161626163", []any{"a", map[any]any{"b": "c"}}, nil, true},
	{"a56161614161626142616361436164614461656145", map[any]any{"a": "A", "b": "B", "c": "C", "d": "D", "e": "E"}, nil, true},
	{"5f42010243030405ff", nil, cbor.ErrUnsupported, false},
	{"7f657374726561646d696e67ff", nil, cbor.ErrUnsupported, false},
	{"9fff", nil, cbor.ErrUnsupported, false},
	{"9f018202039f0405ffff", nil, cbor.ErrUnsupported, false},
	{"9f01820203820405ff", nil, cbor.ErrUnsupported, false},
	{"83018202039f0405ff", nil, cbor.ErrUnsupported, false},
	{"83019f0203ff820405", nil, cbor.ErrUnsupported, false},
	{"9f0102030405060708090a0b0c0d0e0f101112131415161718181819ff", nil, cbor.ErrUnsupported, false},
	{"bf61610161629f0203ffff", nil, cbor.ErrUnsupported, false},
	{"826161bf61626163ff", nil, cbor.ErrUnsupported, false},
	{"bf6346756ef563416d7421ff", nil, cbor.ErrUnsupported, false},
}

func oneThrough25() []any {
	values := make([]any, 25)
	for index := range values {
		values[index] = uint64(index + 1)
	}
	return values
}

func TestRFC8949AppendixA(t *testing.T) {
	if len(appendixA) != 82 {
		t.Fatalf("appendixA has %d vectors, want 82", len(appendixA))
	}
	deterministic := cbor.DecoderOptions{RequireDeterministic: true}
	for _, vector := range appendixA {
		vector := vector
		t.Run(vector.encoded, func(t *testing.T) {
			data, err := hex.DecodeString(vector.encoded)
			if err != nil {
				t.Fatal(err)
			}
			var decoded any
			err = cbor.Unmarshal(data, &decoded)
			if vector.err != nil {
				if !errors.Is(err, vector.err) {
					t.Fatalf("Unmarshal error = %v, want %v", err, vector.err)
				}
				var raw cbor.RawMessage
				if err := cbor.Unmarshal(data, &raw); !errors.Is(err, vector.err) {
					t.Fatalf("RawMessage error = %v, want %v", err, vector.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if !sameValue(decoded, vector.want) {
				t.Fatalf("decoded %#v, want %#v", decoded, vector.want)
			}

			// Every accepted vector survives RawMessage unchanged.
			var raw cbor.RawMessage
			if err := cbor.Unmarshal(data, &raw); err != nil || !bytes.Equal(raw, data) {
				t.Fatalf("RawMessage = %x, %v", raw, err)
			}

			// Deterministic re-encoding reproduces exactly the vectors that
			// RFC 8949 marks as round-tripping, and the deterministic decoder
			// accepts exactly those.
			encoded, err := cbor.CoreDeterministicEncoding().Marshal(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if got := bytes.Equal(encoded, data); got != vector.deterministic {
				t.Fatalf("deterministic encoding %x; identical = %v, want %v", encoded, got, vector.deterministic)
			}
			err = deterministic.Unmarshal(data, &decoded)
			if vector.deterministic && err != nil {
				t.Fatalf("RequireDeterministic rejected: %v", err)
			}
			if !vector.deterministic && !errors.Is(err, cbor.ErrNotDeterministic) {
				t.Fatalf("RequireDeterministic error = %v, want ErrNotDeterministic", err)
			}
		})
	}
}

// sameValue is reflect.DeepEqual except that NaN equals NaN and zeros keep
// their sign.
func sameValue(got, want any) bool {
	switch want := want.(type) {
	case float64:
		got, ok := got.(float64)
		if !ok {
			return false
		}
		if math.IsNaN(want) {
			return math.IsNaN(got)
		}
		return math.Float64bits(got) == math.Float64bits(want)
	case cbor.TaggedValue:
		got, ok := got.(cbor.TaggedValue)
		return ok && got.Number == want.Number && sameValue(got.Value, want.Value)
	}
	return reflect.DeepEqual(got, want)
}
