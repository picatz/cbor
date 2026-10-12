package interop

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	fx "github.com/fxamacker/cbor/v2"
	"github.com/picatz/cbor"
)

// strict configures the reference decoder to reject what RFC 8949 calls
// invalid and what picatz/cbor's profile excludes on purpose: duplicate keys,
// invalid UTF-8 and indefinite lengths.
var strict = func() fx.DecMode {
	mode, err := fx.DecOptions{
		DupMapKey:        fx.DupMapKeyEnforcedAPF,
		IndefLength:      fx.IndefLengthForbidden,
		UTF8:             fx.UTF8RejectInvalid,
		MaxNestedLevels:  256,
		MaxArrayElements: cbor.DefaultMaxValue,
		MaxMapPairs:      cbor.DefaultMaxValue,
	}.DecMode()
	if err != nil {
		panic(err)
	}
	return mode
}()

var diagnostic = func() fx.DiagMode {
	mode, err := fx.DiagOptions{MaxNestedLevels: 256, MaxArrayElements: cbor.DefaultMaxValue, MaxMapPairs: cbor.DefaultMaxValue}.DiagMode()
	if err != nil {
		panic(err)
	}
	return mode
}()

// compare checks one input against the reference implementation:
//
//   - anything picatz/cbor accepts, the reference accepts;
//   - anything only the reference accepts is rejected by picatz/cbor as
//     unsupported or over a limit, never as malformed or invalid;
//   - re-encoding a decoded value, with either encoder configuration, has the
//     same diagnostic notation as the input.
func compare(t *testing.T, data []byte) {
	t.Helper()
	var raw cbor.RawMessage
	ours := cbor.Unmarshal(data, &raw)
	var value any
	reference := strict.Unmarshal(data, &value)
	switch {
	case ours == nil && reference != nil && tagSemantics(reference):
		// picatz/cbor preserves tags without interpreting their content,
		// which RFC 8949 Section 5.3.2 permits for generic decoders; the
		// reference converts tags 0 and 1 to time.Time.
		return
	case ours == nil && reference != nil && strings.Contains(reference.Error(), "duplicate map key") && undefinedKey(data):
		// null and undefined are distinct simple values, but the reference
		// decodes both to a Go nil and then reports them as duplicates.
		return
	case ours == nil && reference != nil:
		t.Fatalf("%x: accepted, reference rejected: %v", data, reference)
	case ours != nil && reference == nil && !errors.Is(ours, cbor.ErrUnsupported) && !errors.Is(ours, cbor.ErrLimit):
		t.Fatalf("%x: rejected (%v), reference accepted %#v", data, ours, value)
	case ours != nil:
		return
	}
	want, err := diagnostic.Diagnose(data)
	if err != nil {
		t.Fatalf("%x: Diagnose: %v", data, err)
	}
	var decoded any
	if err := cbor.Unmarshal(data, &decoded); errors.Is(err, cbor.ErrType) {
		return // e.g. byte-string keys, which map[any]any cannot hold
	} else if err != nil {
		t.Fatalf("%x: Unmarshal into any: %v", data, err)
	}
	for _, options := range []cbor.EncoderOptions{{}, cbor.CoreDeterministicEncoding()} {
		encoded, err := options.Marshal(decoded)
		if err != nil {
			t.Fatalf("%x: re-encoding: %v", data, err)
		}
		got, err := diagnostic.Diagnose(encoded)
		if err != nil {
			t.Fatalf("%x: reference cannot read re-encoding %x: %v", data, encoded, err)
		}
		if got != want && !sameMapEntries(t, data, encoded) {
			t.Fatalf("%x: diagnostic changed\n input:      %s\n re-encoded: %s (%x)", data, want, got, encoded)
		}
	}
}

func tagSemantics(err error) bool {
	message := err.Error()
	return strings.HasPrefix(message, "cbor: tag number ") || strings.Contains(message, "time.Time")
}

func undefinedKey(data []byte) bool {
	notation, err := diagnostic.Diagnose(data)
	return err == nil && strings.Contains(notation, "undefined")
}

// sameMapEntries handles the one expected diagnostic difference: map entry
// order, which the encoder normalizes. It compares the reference's decoded
// values instead.
func sameMapEntries(t *testing.T, input, encoded []byte) bool {
	t.Helper()
	var left, right any
	if strict.Unmarshal(input, &left) != nil || strict.Unmarshal(encoded, &right) != nil {
		return false
	}
	leftDeterministic, errLeft := fx.CoreDetEncOptions().EncMode()
	if errLeft != nil {
		t.Fatal(errLeft)
	}
	first, errFirst := leftDeterministic.Marshal(left)
	second, errSecond := leftDeterministic.Marshal(right)
	return errFirst == nil && errSecond == nil && bytes.Equal(first, second)
}

// appendixA is the RFC 8949 Appendix A example set, taken from the main
// module's conformance table by encoding only.
var appendixA = []string{
	"00", "01", "0a", "17", "1818", "1819", "1864", "1903e8", "1a000f4240", "1b000000e8d4a51000",
	"1bffffffffffffffff", "c249010000000000000000", "3bffffffffffffffff", "c349010000000000000000",
	"20", "29", "3863", "3903e7", "f90000", "f98000", "f93c00", "fb3ff199999999999a", "f93e00",
	"f97bff", "fa47c35000", "fa7f7fffff", "fb7e37e43c8800759c", "f90001", "f90400", "f9c400",
	"fbc010666666666666", "f97c00", "f97e00", "f9fc00", "fa7f800000", "fa7fc00000", "faff800000",
	"fb7ff0000000000000", "fb7ff8000000000000", "fbfff0000000000000", "f4", "f5", "f6", "f7", "f0",
	"f818", "f8ff", "c074323031332d30332d32315432303a30343a30305a", "c11a514b67b0", "c1fb41d452d9ec200000",
	"d74401020304", "d818456449455446", "d82076687474703a2f2f7777772e6578616d706c652e636f6d", "40",
	"4401020304", "60", "6161", "6449455446", "62225c", "62c3bc", "63e6b0b4", "64f0908591", "80",
	"83010203", "8301820203820405", "98190102030405060708090a0b0c0d0e0f101112131415161718181819",
	"a0", "a201020304", "a26161016162820203", "826161a161626163", "a56161614161626142616361436164614461656145",
	"5f42010243030405ff", "7f657374726561646d696e67ff", "9fff", "9f018202039f0405ffff", "9f01820203820405ff",
	"83018202039f0405ff", "83019f0203ff820405", "9f0102030405060708090a0b0c0d0e0f101112131415161718181819ff",
	"bf61610161629f0203ffff", "826161bf61626163ff", "bf6346756ef563416d7421ff",
}

// extra covers FIDO-shaped and boundary inputs beyond Appendix A.
var extra = []string{
	"a301020304a10518",               // truncated nested map
	"a2010201f5",                     // duplicate key with different values
	"a2f93c00f5fb3ff0000000000000f4", // 1.0 as binary16 and binary64 keys
	"a2f90000f5f98000f4",             // +0.0 and -0.0 keys
	"a2616101410102",                 // text and byte-string keys with equal content
	"a1f97e00f5",                     // NaN key
	"a1810000",                       // array key
	"62c328",                         // invalid UTF-8
	"1801",                           // non-minimal integer
	"5f4101ff",                       // indefinite byte string
	"ff",                             // lone break
	"1c",                             // reserved additional information
	"c0c0c0c000",                     // nested tags
	"d9d9f700",                       // self-described CBOR tag
	"a50102032620012158200000000000000000000000000000000000000000000000000000000000000000225820" +
		"0000000000000000000000000000000000000000000000000000000000000000", // COSE EC2 key
}

func TestDifferentialVectors(t *testing.T) {
	for _, vectors := range [][]string{appendixA, extra} {
		for _, encoded := range vectors {
			data, err := hex.DecodeString(encoded)
			if err != nil {
				t.Fatal(err)
			}
			t.Run(encoded, func(t *testing.T) { compare(t, data) })
		}
	}
}

func FuzzDifferential(f *testing.F) {
	for _, vectors := range [][]string{appendixA, extra} {
		for _, encoded := range vectors {
			data, _ := hex.DecodeString(encoded)
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			return
		}
		compare(t, data)
	})
}

// TestDeterministicEncodingMatchesReference compares RFC 8949 core
// deterministic output byte for byte with the reference encoder for values
// both libraries represent the same way.
func TestDeterministicEncodingMatchesReference(t *testing.T) {
	reference, err := fx.CoreDetEncOptions().EncMode()
	if err != nil {
		t.Fatal(err)
	}
	values := []any{
		map[string]any{"z": 1, "aa": 2, "b": []any{1.5, 100000.0, 1.1, -0.0}},
		map[int64]string{-25: "neg", 24: "two-byte", 0: "zero", -1: "minus one"},
		map[any]bool{uint64(24): true, "": true, "a": true, int64(-1): true},
		[]float64{0, 65504, 65505, 5.960464477539063e-8, 3.4028234663852886e+38, 1e300},
		[]byte{1, 2, 3},
		struct {
			Alg  int    `cbor:"3,keyasint"`
			Kty  int    `cbor:"1,keyasint"`
			Crv  int    `cbor:"-1,keyasint"`
			Name string `cbor:"name"`
		}{Alg: -7, Kty: 2, Crv: 1, Name: "p256"},
	}
	for _, value := range values {
		ours, err := cbor.CoreDeterministicEncoding().Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		theirs, err := reference.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(ours, theirs) {
			t.Errorf("%#v:\n ours   %x\n theirs %x", value, ours, theirs)
		}
	}
}
