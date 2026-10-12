package cbor_test

import (
	"bytes"
	"encoding/hex"
	"io"
	"math"
	"reflect"
	"testing"
	"testing/iotest"

	"github.com/picatz/cbor"
)

var fuzzOptions = cbor.DecoderOptions{
	MaxEncodedBytes: 8192, MaxNestedLevels: 16, MaxArrayElements: 256,
	MaxMapPairs: 128, MaxBytes: 8192, MaxStringBytes: 8192,
}

func addSeeds(f *testing.F) {
	for _, vector := range appendixA {
		data, err := hex.DecodeString(vector.encoded)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Add(authenticatorGetInfo)
	f.Add(cwtClaims)
	f.Add(append(append([]byte{}, cwtClaims...), authenticatorGetInfo...))
}

// FuzzRoundTrip checks that every accepted item re-encodes, under both
// encoder configurations, to an item that decodes to the same value, and that
// core deterministic encoding is a fixed point accepted by
// RequireDeterministic.
func FuzzRoundTrip(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		var value any
		if err := fuzzOptions.Unmarshal(data, &value); err != nil {
			return
		}
		for _, options := range []cbor.EncoderOptions{{}, cbor.CoreDeterministicEncoding()} {
			encoded, err := options.Marshal(value)
			if err != nil {
				t.Fatalf("%+v: re-encoding %#v: %v", options, value, err)
			}
			var again any
			if err := fuzzOptions.Unmarshal(encoded, &again); err != nil {
				t.Fatalf("%+v: decoding %x: %v", options, encoded, err)
			}
			if !deepSame(value, again) {
				t.Fatalf("%+v: %#v became %#v", options, value, again)
			}
		}
		deterministic, _ := cbor.CoreDeterministicEncoding().Marshal(value)
		strict := fuzzOptions
		strict.RequireDeterministic = true
		var canonical any
		if err := strict.Unmarshal(deterministic, &canonical); err != nil {
			t.Fatalf("RequireDeterministic rejected %x: %v", deterministic, err)
		}
		if again, _ := cbor.CoreDeterministicEncoding().Marshal(canonical); !bytes.Equal(again, deterministic) {
			t.Fatalf("deterministic encoding is not a fixed point: %x then %x", deterministic, again)
		}
		// Input the strict decoder accepts must already be deterministic.
		if strict.Unmarshal(data, &canonical) == nil && !bytes.Equal(data, deterministic) {
			t.Fatalf("RequireDeterministic accepted %x, deterministic form is %x", data, deterministic)
		}
	})
}

// FuzzStream checks that Decoder splits a stream into the same items whether
// reads are whole or one byte at a time, that every item is consistent with
// Unmarshal, and that every truncation reports io.ErrUnexpectedEOF.
func FuzzStream(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 1024 {
			return
		}
		whole := cbor.NewDecoderWithOptions(bytes.NewReader(data), fuzzOptions)
		bytewise := cbor.NewDecoderWithOptions(iotest.OneByteReader(bytes.NewReader(data)), fuzzOptions)
		consumed := 0
		for {
			var first, second cbor.RawMessage
			firstErr := whole.Decode(&first)
			secondErr := bytewise.Decode(&second)
			if (firstErr == nil) != (secondErr == nil) || !bytes.Equal(first, second) {
				t.Fatalf("reader shape changed result: %x %v / %x %v", first, firstErr, second, secondErr)
			}
			if firstErr != nil {
				if firstErr == io.EOF && consumed != len(data) {
					t.Fatalf("io.EOF after %d of %d bytes", consumed, len(data))
				}
				return
			}
			if !bytes.Equal(first, data[consumed:consumed+len(first)]) {
				t.Fatal("item is not the next slice of the stream")
			}
			consumed += len(first)
			var raw cbor.RawMessage
			if err := fuzzOptions.Unmarshal(first, &raw); err != nil || !bytes.Equal(raw, first) {
				t.Fatalf("Unmarshal disagrees with Decode on %x: %v", first, err)
			}
			for cut := 1; cut < len(first); cut++ {
				if err := fuzzOptions.Unmarshal(first[:cut], &raw); err != io.ErrUnexpectedEOF {
					t.Fatalf("Unmarshal of %x truncated at %d: %v", first, cut, err)
				}
				decoder := cbor.NewDecoderWithOptions(iotest.OneByteReader(bytes.NewReader(first[:cut])), fuzzOptions)
				if err := decoder.Decode(&raw); err != io.ErrUnexpectedEOF {
					t.Fatalf("Decode of %x truncated at %d: %v", first, cut, err)
				}
			}
		}
	})
}

// FuzzShortestFloat checks FloatShortest against the decoder for every float64
// bit pattern: the value survives exactly, NaNs become 0xf97e00, and no
// shorter width would have preserved it.
func FuzzShortestFloat(f *testing.F) {
	for _, bits := range []uint64{0, 1 << 63, 0x3ff0000000000000, 0x7ff0000000000000, 0x7ff8000000000001, 0x3e70000000000000, 0x3f1ff00000000000} {
		f.Add(bits)
	}
	shortest := cbor.EncoderOptions{Floats: cbor.FloatShortest}
	f.Fuzz(func(t *testing.T, bits uint64) {
		number := math.Float64frombits(bits)
		encoded, err := shortest.Marshal(number)
		if err != nil {
			t.Fatal(err)
		}
		var decoded float64
		if err := cbor.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if math.IsNaN(number) {
			if !bytes.Equal(encoded, []byte{0xf9, 0x7e, 0x00}) {
				t.Fatalf("NaN encoded as %x", encoded)
			}
			return
		}
		if math.Float64bits(decoded) != bits {
			t.Fatalf("%x became %x via %x", bits, math.Float64bits(decoded), encoded)
		}
		if encoded[0] != 0xf9 && float64(float32(number)) == number && fitsHalf(number) {
			t.Fatalf("%v encoded as %x but fits binary16", number, encoded)
		}
		if encoded[0] == 0xfb && float64(float32(number)) == number {
			t.Fatalf("%v encoded as %x but fits binary32", number, encoded)
		}
	})
}

// fitsHalf is an independent binary16 exactness check based on the value's
// binary exponent and significant bits.
func fitsHalf(number float64) bool {
	if number == 0 || math.IsInf(number, 0) {
		return true
	}
	fraction, exponent := math.Frexp(math.Abs(number)) // number = fraction * 2^exponent, fraction in [0.5, 1)
	if exponent > 16 {
		return false
	}
	// binary16 has 11 significant bits for normal values (exponent >= -13) and
	// fewer for subnormals, whose least significant bit is 2^-24.
	bits := 11
	if exponent < -13 {
		bits = 11 - (-13 - exponent)
	}
	if bits <= 0 {
		return false
	}
	scaled := math.Ldexp(fraction, bits)
	return scaled == math.Trunc(scaled)
}

// deepSame compares decoded values, treating NaN as equal to NaN and keeping
// the sign of zero.
func deepSame(left, right any) bool {
	switch left := left.(type) {
	case float64:
		right, ok := right.(float64)
		return ok && (math.Float64bits(left) == math.Float64bits(right) || math.IsNaN(left) && math.IsNaN(right))
	case []any:
		right, ok := right.([]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for index := range left {
			if !deepSame(left[index], right[index]) {
				return false
			}
		}
		return true
	case map[any]any:
		right, ok := right.(map[any]any)
		if !ok || len(left) != len(right) {
			return false
		}
		for key, value := range left {
			other, ok := right[key]
			if !ok || !deepSame(value, other) {
				return false
			}
		}
		return true
	case cbor.TaggedValue:
		right, ok := right.(cbor.TaggedValue)
		return ok && left.Number == right.Number && deepSame(left.Value, right.Value)
	}
	return reflect.DeepEqual(left, right)
}
