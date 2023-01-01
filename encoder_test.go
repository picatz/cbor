package cbor_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/picatz/cbor"
)

func TestEncodeString(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode("hello world")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())
}

func TestEncodeInt(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode(1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())
}

func TestEncodeUint(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode(uint(1))
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())
}

func TestEncodeFloat(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode(1.0)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())
}

func TestEncodeBool(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode(true)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())
}

func TestEncodeArray(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode([]int{1, 2})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())
}

func TestEncodeMap(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode(map[string]int{"one": 1, "two": 2})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())

	// decode
	dec := cbor.NewDecoder(&buf)
	var m map[string]int
	err = dec.Decode(&m)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%#v\n", m)
}

type testStruct struct {
	One int
	Two int
}

// TODO: decode struct
func TestEncodeStruct(t *testing.T) {
	var buf bytes.Buffer
	enc := cbor.NewEncoder(&buf)
	err := enc.Encode(testStruct{One: 1, Two: 2})
	if err != nil {
		t.Fatal(err)
	}
	fmt.Printf("%x\n", buf.Bytes())
}
