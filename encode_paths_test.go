package cbor_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

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
