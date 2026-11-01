package cbor_test

import (
	"bytes"
	"errors"
	"math/big"
	"reflect"
	"testing"

	"github.com/picatz/cbor"
)

// FuzzDiagnose checks that Diagnose accepts exactly what Unmarshal accepts
// into RawMessage, fails with the same error, and never panics.
func FuzzDiagnose(f *testing.F) {
	addSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		var raw cbor.RawMessage
		decodeErr := fuzzOptions.Unmarshal(data, &raw)
		notation, err := fuzzOptions.Diagnose(data)
		if (err == nil) != (decodeErr == nil) || err != nil && err.Error() != decodeErr.Error() {
			t.Fatalf("%x: Diagnose error %v, Unmarshal error %v", data, err, decodeErr)
		}
		if err == nil && notation == "" {
			t.Fatalf("%x: empty notation", data)
		}
	})
}

// schema exercises the struct features together: struct-wide integer keys,
// required fields, embedded structs, big integers, tags and raw values.
type schema struct {
	schemaHeader
	_        struct{}                  `cbor:",keyasint"`
	Count    uint16                    `cbor:"-1,required"`
	Big      *big.Int                  `cbor:"-2,omitempty"`
	Tagged   cbor.Tag[[]int8]          `cbor:"-3,omitzero"`
	Extra    map[int64]cbor.RawMessage `cbor:"-4,omitempty"`
	Fraction float32                   `cbor:"-5,omitempty"`
}

type schemaHeader struct {
	_     struct{} `cbor:",keyasint"`
	Kind  int64    `cbor:"1,required"`
	Label string   `cbor:"2,omitempty"`
	Flag  bool     `cbor:"3,omitempty"`
}

// FuzzSchema decodes into a strict struct. Accepted input re-encodes
// deterministically to bytes that decode to the same value, and rejected
// input leaves the destination unchanged.
func FuzzSchema(f *testing.F) {
	for _, value := range []schema{
		{schemaHeader: schemaHeader{Kind: 1, Label: "a"}, Count: 2},
		{schemaHeader: schemaHeader{Kind: -1, Flag: true}, Count: 65535, Big: new(big.Int).Lsh(big.NewInt(1), 70)},
		{Count: 1, Tagged: cbor.Tag[[]int8]{Number: 99, Content: []int8{-128, 127}}, Extra: map[int64]cbor.RawMessage{7: {0xf6}}},
		{Count: 1, Fraction: 1.5},
	} {
		encoded, err := cbor.CoreDeterministicEncoding().Marshal(value)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(encoded)
	}
	addSeeds(f)
	strict := fuzzOptions
	strict.RejectUnknownFields = true
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, options := range []cbor.DecoderOptions{fuzzOptions, strict} {
			sentinel := schema{schemaHeader: schemaHeader{Label: "sentinel"}}
			value := sentinel
			if err := options.Unmarshal(data, &value); err != nil {
				if !reflect.DeepEqual(value, sentinel) {
					t.Fatalf("%x: destination changed on error %v", data, err)
				}
				var zero schema
				if errZero := options.Unmarshal(data, &zero); !reflect.DeepEqual(zero, schema{}) || errZero == nil {
					t.Fatalf("%x: zero destination changed on error: %+v, %v", data, zero, errZero)
				}
				continue
			}
			encoded, err := cbor.CoreDeterministicEncoding().Marshal(value)
			if err != nil {
				if errors.Is(err, cbor.ErrNotDeterministic) {
					continue // a RawMessage in Extra that is not deterministic
				}
				t.Fatalf("%x: re-encoding %+v: %v", data, value, err)
			}
			var again schema
			if err := (cbor.DecoderOptions{RejectUnknownFields: true, RequireDeterministic: true}).Unmarshal(encoded, &again); err != nil {
				t.Fatalf("%x: re-encoding %x rejected: %v", data, encoded, err)
			}
			if reencoded, _ := cbor.CoreDeterministicEncoding().Marshal(again); !bytes.Equal(reencoded, encoded) {
				t.Fatalf("%x: deterministic encoding is not a fixed point: %x then %x", data, encoded, reencoded)
			}
		}
	})
}
