package cbor_test

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/picatz/cbor"
)

// TestOptionalFieldHeaders checks the map header of structs with omitted
// fields on both sides of the one-byte header limit: fewer than 24 fields
// patch a reserved byte, more select fields first.
func TestOptionalFieldHeaders(t *testing.T) {
	for _, count := range []int{1, 23, 24, 30} {
		fields := make([]reflect.StructField, count)
		for index := range fields {
			fields[index] = reflect.StructField{
				Name: fmt.Sprintf("F%d", index),
				Type: reflect.TypeFor[int](),
				Tag:  reflect.StructTag(fmt.Sprintf(`cbor:"%d,keyasint,omitempty"`, index)),
			}
		}
		kind := reflect.StructOf(fields)
		for _, present := range []int{0, 1, count / 2, count} {
			value := reflect.New(kind).Elem()
			want := make(map[uint64]int64, present)
			for index := range present {
				value.Field(index).SetInt(int64(index + 1))
				want[uint64(index)] = int64(index + 1)
			}
			encoded, err := cbor.CoreDeterministicEncoding().Marshal(value.Interface())
			if err != nil {
				t.Fatalf("%d fields, %d present: %v", count, present, err)
			}
			var decoded map[uint64]int64
			if err := (cbor.DecoderOptions{RequireDeterministic: true}).Unmarshal(encoded, &decoded); err != nil {
				t.Fatalf("%d fields, %d present: %x: %v", count, present, encoded, err)
			}
			if len(decoded) != present || present > 0 && !reflect.DeepEqual(decoded, want) {
				t.Fatalf("%d fields, %d present: decoded %v", count, present, decoded)
			}
		}
	}
}

// TestMapIterationValuesAcrossTypes checks that maps of different types at
// one depth, and nested maps of one type, each encode with their own key
// and element values.
func TestMapIterationValuesAcrossTypes(t *testing.T) {
	value := []any{
		map[string]int{"a": 1, "b": 2},
		map[int]string{1: "x"},
		map[string]map[string]bool{"outer": {"inner": true}},
		map[string]int{"c": 3},
	}
	encoded, err := cbor.CoreDeterministicEncoding().Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	notation, err := cbor.Diagnose(encoded)
	want := `[{"a": 1, "b": 2}, {1: "x"}, {"outer": {"inner": true}}, {"c": 3}]`
	if err != nil || notation != want {
		t.Fatalf("encoded %s, %v; want %s", notation, err, want)
	}
	type recursive map[string]recursive
	nested := recursive{"a": {"b": {"c": nil}}, "d": {}}
	if encoded, err = cbor.CoreDeterministicEncoding().Marshal(nested); err != nil {
		t.Fatal(err)
	}
	if notation, _ = cbor.Diagnose(encoded); notation != `{"a": {"b": {"c": null}}, "d": {}}` {
		t.Fatalf("recursive map encoded %s", notation)
	}
	// A failed map encoding leaves no state behind for the next one.
	if _, err := cbor.Marshal(map[string]string{"bad": "\xff"}); err == nil || !strings.Contains(err.Error(), "UTF-8") {
		t.Fatalf("invalid UTF-8 value: %v", err)
	}
	if encoded, err = cbor.Marshal(map[string]string{"ok": "yes"}); err != nil {
		t.Fatal(err)
	}
	if notation, _ = cbor.Diagnose(encoded); notation != `{"ok": "yes"}` {
		t.Fatalf("after a failure, encoded %s", notation)
	}
}

// pointerZero reports zero only through a pointer receiver.
type pointerZero struct{ n int }

func (value *pointerZero) IsZero() bool { return value.n == 0 || value.n == 42 }

// TestOmitZeroMethods checks omitzero for value-receiver and
// pointer-receiver IsZero methods, nil pointers and interfaces, and fields
// reached with and without an addressable struct.
func TestOmitZeroMethods(t *testing.T) {
	type fields struct {
		When    time.Time    `cbor:"1,keyasint,omitzero"`
		Pointer pointerZero  `cbor:"2,keyasint,omitzero"`
		Nil     *time.Time   `cbor:"3,keyasint,omitzero"`
		Iface   fmt.Stringer `cbor:"4,keyasint,omitzero"`
		Count   int          `cbor:"5,keyasint,omitzero"`
	}
	// pointerZero{n: 42} is zero by its method, but only an addressable
	// field can call it; otherwise its Go zero value decides.
	value := fields{Pointer: pointerZero{n: 42}, Count: 1}
	addressable, err := cbor.Marshal(&value)
	if err != nil {
		t.Fatal(err)
	}
	if notation, _ := cbor.Diagnose(addressable); notation != "{5: 1}" {
		t.Fatalf("through a pointer: %s", notation)
	}
	copied, err := cbor.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if notation, _ := cbor.Diagnose(copied); notation != "{2: {}, 5: 1}" {
		t.Fatalf("by value: %s", notation)
	}
	// A time that is not zero is kept, by value and through a pointer.
	value.When = time.Unix(1, 0)
	for _, encode := range []any{value, &value} {
		encoded, err := cbor.Marshal(encode)
		if err != nil {
			t.Fatal(err)
		}
		var keys map[int]cbor.RawMessage
		if err := cbor.Unmarshal(encoded, &keys); err != nil {
			t.Fatal(err)
		}
		if _, ok := keys[1]; !ok {
			t.Fatalf("non-zero time omitted: %x", encoded)
		}
	}
}

// TestConcurrentMixedTypes checks that the per-state codec caches stay
// correct when goroutines interleave many types; run it with -race.
func TestConcurrentMixedTypes(t *testing.T) {
	type small struct {
		A int `cbor:"1,keyasint"`
	}
	type text struct {
		Name string `cbor:"name"`
	}
	values := []any{
		small{A: 1}, &small{A: 2}, text{Name: "x"}, map[string]int{"k": 3},
		[]string{"a", "b"}, uint64(7), map[int]small{1: {A: 4}}, []any{small{A: 5}, "y"},
	}
	encodings := make([][]byte, len(values))
	for index, value := range values {
		encoded, err := cbor.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		encodings[index] = encoded
	}
	var group sync.WaitGroup
	for worker := range 16 {
		group.Go(func() {
			for round := range 200 {
				index := (worker + round) % len(values)
				encoded, err := cbor.Marshal(values[index])
				if err != nil || !bytes.Equal(encoded, encodings[index]) {
					t.Errorf("Marshal(%#v) = %x, %v; want %x", values[index], encoded, err, encodings[index])
					return
				}
				decoded := reflect.New(reflect.TypeOf(values[index]))
				if err := cbor.Unmarshal(encoded, decoded.Interface()); err != nil {
					t.Errorf("Unmarshal into %T: %v", values[index], err)
					return
				}
				if again, err := cbor.Marshal(decoded.Elem().Interface()); err != nil || !bytes.Equal(again, encoded) {
					t.Errorf("%T round trip = %x, %v; want %x", values[index], again, err, encoded)
					return
				}
			}
		})
	}
	group.Wait()
}
