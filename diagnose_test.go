package cbor_test

import (
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/picatz/cbor"
)

// appendixADiagnostics is the diagnostic notation RFC 8949 Appendix A gives
// for each example this package accepts, verbatim.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#appendix-A
var appendixADiagnostics = map[string]string{
	"00": "0", "01": "1", "0a": "10", "17": "23", "1818": "24", "1819": "25", "1864": "100",
	"1903e8": "1000", "1a000f4240": "1000000", "1b000000e8d4a51000": "1000000000000",
	"1bffffffffffffffff":     "18446744073709551615",
	"c249010000000000000000": "2(h'010000000000000000')",
	"3bffffffffffffffff":     "-18446744073709551616",
	"c349010000000000000000": "3(h'010000000000000000')",
	"20":                     "-1", "29": "-10", "3863": "-100", "3903e7": "-1000",
	"f90000": "0.0", "f98000": "-0.0", "f93c00": "1.0", "fb3ff199999999999a": "1.1",
	"f93e00": "1.5", "f97bff": "65504.0", "fa47c35000": "100000.0",
	"fa7f7fffff": "3.4028234663852886e+38", "fb7e37e43c8800759c": "1.0e+300",
	"f90001": "5.960464477539063e-8", "f90400": "0.00006103515625", "f9c400": "-4.0",
	"fbc010666666666666": "-4.1", "f97c00": "Infinity", "f97e00": "NaN", "f9fc00": "-Infinity",
	"fa7f800000": "Infinity", "fa7fc00000": "NaN", "faff800000": "-Infinity",
	"fb7ff0000000000000": "Infinity", "fb7ff8000000000000": "NaN", "fbfff0000000000000": "-Infinity",
	"f4": "false", "f5": "true", "f6": "null", "f7": "undefined", "f0": "simple(16)", "f8ff": "simple(255)",
	"c074323031332d30332d32315432303a30343a30305a": `0("2013-03-21T20:04:00Z")`,
	"c11a514b67b0":         "1(1363896240)",
	"c1fb41d452d9ec200000": "1(1363896240.5)",
	"d74401020304":         "23(h'01020304')",
	"d818456449455446":     "24(h'6449455446')",
	"d82076687474703a2f2f7777772e6578616d706c652e636f6d": `32("http://www.example.com")`,
	"40":               "h''",
	"4401020304":       "h'01020304'",
	"60":               `""`,
	"6161":             `"a"`,
	"6449455446":       `"IETF"`,
	"62225c":           `"\"\\"`,
	"62c3bc":           `"ü"`,
	"63e6b0b4":         `"水"`,
	"64f0908591":       `"𐅑"`,
	"80":               "[]",
	"83010203":         "[1, 2, 3]",
	"8301820203820405": "[1, [2, 3], [4, 5]]",
	"98190102030405060708090a0b0c0d0e0f101112131415161718181819": "[1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25]",
	"a0":                 "{}",
	"a201020304":         "{1: 2, 3: 4}",
	"a26161016162820203": `{"a": 1, "b": [2, 3]}`,
	"826161a161626163":   `["a", {"b": "c"}]`,
	"a56161614161626142616361436164614461656145": `{"a": "A", "b": "B", "c": "C", "d": "D", "e": "E"}`,
}

func TestDiagnoseAppendixA(t *testing.T) {
	for encoded, want := range appendixADiagnostics {
		data, err := hex.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		got, err := cbor.Diagnose(data)
		if err != nil || got != want {
			t.Errorf("Diagnose(%s) = %q, %v; want %q", encoded, got, err, want)
		}
	}
}

func TestDiagnose(t *testing.T) {
	for _, test := range []struct{ encoded, want string }{
		{"a201260442 3131", "{1: -7, 4: h'3131'}"},
		{"fa3fc00000", "1.5"},
		{"fb3eb0c6f7a0b5ed8d", "0.000001"},
		{"fb3e7ad7f29abcaf48", "1.0e-7"},
		{"fb4415af1d78b58c40", "100000000000000000000.0"},
		{"fb444b1ae4d6e2ef50", "1.0e+21"},
		{"fb8000000000000000", "-0.0"},
		{"6b0a0d091f7f2f0022e280a8", `"\n\r\t\u001f\u007f/\u0000\"` + " \""},
		{"f820", "simple(32)"},
		{"d9d9f7a0", "55799({})"},
		{"3b7fffffffffffffff", "-9223372036854775808"},
	} {
		data, err := hex.DecodeString(strings.ReplaceAll(test.encoded, " ", ""))
		if err != nil {
			t.Fatal(err)
		}
		if got, err := cbor.Diagnose(data); err != nil || got != test.want {
			t.Errorf("Diagnose(%s) = %q, %v; want %q", test.encoded, got, err, test.want)
		}
	}
}

func TestDiagnoseRejects(t *testing.T) {
	for _, test := range []struct {
		encoded string
		options cbor.DecoderOptions
		kind    error
	}{
		{"0001", cbor.DecoderOptions{}, cbor.ErrMalformed},
		{"a20102010 3", cbor.DecoderOptions{}, cbor.ErrInvalid},
		{"9f01ff", cbor.DecoderOptions{}, cbor.ErrUnsupported},
		{"8301020 3", cbor.DecoderOptions{MaxArrayElements: 2}, cbor.ErrLimit},
		{"1801", cbor.DecoderOptions{RequireDeterministic: true}, cbor.ErrNotDeterministic},
		{"c101", cbor.DecoderOptions{RejectTags: true}, cbor.ErrUnsupported},
	} {
		data, err := hex.DecodeString(strings.ReplaceAll(test.encoded, " ", ""))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := test.options.Diagnose(data); !errors.Is(err, test.kind) {
			t.Errorf("Diagnose(%s) error = %v, want %v", test.encoded, err, test.kind)
		}
	}
	if _, err := cbor.Diagnose(nil); err == nil {
		t.Error("Diagnose(nil) succeeded")
	}
}
