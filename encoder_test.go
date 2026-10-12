package cbor_test

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"

	"github.com/picatz/cbor"
)

type testStruct struct {
	One int
	Two int
}

func TestEncoderEncode(t *testing.T) {
	for _, test := range []struct {
		name  string
		value any
		want  string
	}{
		{"string", "hello world", "6b68656c6c6f20776f726c64"},
		{"int", 1, "01"},
		{"uint", uint(1), "01"},
		{"float", 1.0, "fb3ff0000000000000"},
		{"bool", true, "f5"},
		{"array", []int{1, 2}, "820102"},
		{"map", map[string]int{"one": 1, "two": 2}, "a2636f6e65016374776f02"},
		{"struct", testStruct{One: 1, Two: 2}, "a2634f6e65016354776f02"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var buffer bytes.Buffer
			if err := cbor.NewEncoder(&buffer).Encode(test.value); err != nil {
				t.Fatal(err)
			}
			if got := hex.EncodeToString(buffer.Bytes()); got != test.want {
				t.Fatalf("got %s, want %s", got, test.want)
			}
			decoded := reflect.New(reflect.TypeOf(test.value))
			if err := cbor.NewDecoder(&buffer).Decode(decoded.Interface()); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.Elem().Interface(), test.value) {
				t.Fatalf("round trip produced %#v", decoded.Elem().Interface())
			}
		})
	}
}

func TestEncoderWithOptions(t *testing.T) {
	var buffer bytes.Buffer
	encoder := cbor.NewEncoderWithOptions(&buffer, cbor.CoreDeterministicEncoding())
	if err := encoder.Encode(map[any]float64{uint64(24): 1.5, "": 0.5}); err != nil {
		t.Fatal(err)
	}
	if got := hex.EncodeToString(buffer.Bytes()); got != "a21818f93e0060f93800" {
		t.Fatalf("got %s", got)
	}
	if err := cbor.NewEncoderWithOptions(&buffer, cbor.EncoderOptions{Sort: 7}).Encode(1); err == nil {
		t.Fatal("accepted invalid options")
	}
}
