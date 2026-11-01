package cbor_test

import (
	"bytes"
	"errors"
	"io"
	"reflect"
	"slices"
	"testing"

	"github.com/picatz/cbor"
)

// calls records every UnmarshalCBOR invocation, in order, so tests can
// assert that application code runs only for items that decode completely.
var calls []string

// recorder is an Unmarshaler with an observable side effect.
type recorder struct{ Text string }

func (value *recorder) UnmarshalCBOR(data []byte) error {
	var text string
	if err := cbor.Unmarshal(data, &text); err != nil {
		return err
	}
	calls = append(calls, text)
	value.Text = text
	return nil
}

// refusing is an Unmarshaler that always fails.
type refusing struct{}

func (*refusing) UnmarshalCBOR([]byte) error {
	calls = append(calls, "refused")
	return errRefused
}

var errRefused = errors.New("refused")

// panicking is an Unmarshaler that panics.
type panicking struct{}

func (*panicking) UnmarshalCBOR([]byte) error { panic("unmarshal panic") }

type withRecorders struct {
	First  recorder            `cbor:"1,keyasint"`
	Many   []recorder          `cbor:"2,keyasint"`
	ByName map[string]recorder `cbor:"3,keyasint"`
	Tagged cbor.Tag[*recorder] `cbor:"4,keyasint"`
	Count  int                 `cbor:"5,keyasint"`
}

// TestUnmarshalerDeferredUntilValid checks that no UnmarshalCBOR method runs,
// and the destination is unchanged, when anything after an Unmarshaler fails.
func TestUnmarshalerDeferredUntilValid(t *testing.T) {
	// {1: "a", 2: ["b"], 3: {"k": "c"}, 4: 7("d"), 5: ...} with each failure
	// placed after every Unmarshaler.
	prefix := "a5" + "016161" + "02816162" + "03a1616b6163" + "04c76164"
	for _, test := range []struct {
		name, encoded string
		kind          error
	}{
		{"later type error", prefix + "05" + "6178", cbor.ErrType},
		{"later duplicate key", "a6" + prefix[2:] + "0501" + "016178", cbor.ErrInvalid},
		{"later invalid UTF-8", prefix + "05" + "62c328", cbor.ErrInvalid},
		{"later reserved header", prefix + "05" + "1c", cbor.ErrMalformed},
		{"later indefinite length", prefix + "05" + "9fff", cbor.ErrUnsupported},
		{"trailing data", prefix + "0501" + "00", cbor.ErrMalformed},
		{"truncated", prefix + "05", io.ErrUnexpectedEOF},
	} {
		t.Run(test.name, func(t *testing.T) {
			data := mustHex(test.encoded)
			for _, destination := range []withRecorders{{}, {Count: 42, First: recorder{"kept"}}} {
				calls = nil
				value := destination
				err := cbor.Unmarshal(data, &value)
				if !errors.Is(err, test.kind) {
					t.Fatalf("error = %v, want %v", err, test.kind)
				}
				if len(calls) != 0 {
					t.Fatalf("UnmarshalCBOR ran on invalid input: %q", calls)
				}
				if !reflect.DeepEqual(value, destination) {
					t.Fatalf("destination changed: %+v, want %+v", value, destination)
				}
			}
		})
	}

	// The valid item runs every Unmarshaler once, in document order.
	calls = nil
	var value withRecorders
	if err := cbor.Unmarshal(mustHex(prefix+"0501"), &value); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(calls, []string{"a", "b", "c", "d"}) {
		t.Fatalf("calls = %q, want [a b c d]", calls)
	}
	if value.First.Text != "a" || value.Many[0].Text != "b" || value.ByName["k"].Text != "c" ||
		value.Tagged.Number != 7 || value.Tagged.Content.Text != "d" || value.Count != 1 {
		t.Fatalf("decoded %+v", value)
	}
}

// TestUnmarshalerDeferredInStreams checks Decoder.Decode and Decoder.All,
// which read without look-ahead.
func TestUnmarshalerDeferredInStreams(t *testing.T) {
	calls = nil
	var list []recorder
	// ["a", "b", <reserved header>]
	err := cbor.NewDecoder(bytes.NewReader(mustHex("8361616162" + "1c"))).Decode(&list)
	if !errors.Is(err, cbor.ErrMalformed) || len(calls) != 0 || list != nil {
		t.Fatalf("malformed stream: %v, calls %q, list %v", err, calls, list)
	}

	calls = nil
	// ["a", "b", ...] truncated: the stream ends inside the array.
	err = cbor.NewDecoder(bytes.NewReader(mustHex("8361616162"))).Decode(&list)
	if !errors.Is(err, io.ErrUnexpectedEOF) || len(calls) != 0 || list != nil {
		t.Fatalf("truncated stream: %v, calls %q, list %v", err, calls, list)
	}

	calls = nil
	for message, err := range cbor.NewDecoder(bytes.NewReader(mustHex("6161" + "1c"))).All() {
		if err != nil {
			break
		}
		var value recorder
		if err := cbor.Unmarshal(message, &value); err != nil {
			t.Fatal(err)
		}
	}
	if !slices.Equal(calls, []string{"a"}) {
		t.Fatalf("calls = %q, want [a]", calls)
	}
}

// TestUnmarshalerErrorLeavesDestination checks that an UnmarshalCBOR error
// fails the decode and leaves the destination unchanged. Earlier
// Unmarshalers in the same item may already have run.
func TestUnmarshalerErrorLeavesDestination(t *testing.T) {
	type mixed struct {
		Before recorder `cbor:"1,keyasint"`
		Refuse refusing `cbor:"2,keyasint"`
		After  int      `cbor:"3,keyasint"`
	}
	calls = nil
	original := mixed{After: 9}
	value := original
	err := cbor.Unmarshal(mustHex("a3016161020003"+"01"), &value)
	if !errors.Is(err, errRefused) {
		t.Fatalf("error = %v, want errRefused", err)
	}
	if !reflect.DeepEqual(value, original) {
		t.Fatalf("destination changed: %+v", value)
	}
	if !slices.Contains(calls, "refused") {
		t.Fatalf("calls = %q", calls)
	}
}

// TestUnmarshalerPanicResetsState checks that a panicking UnmarshalCBOR does
// not leave queued operations behind for the next decode.
func TestUnmarshalerPanicResetsState(t *testing.T) {
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("UnmarshalCBOR panic did not propagate")
			}
		}()
		var values struct {
			First recorder  `cbor:"1,keyasint"`
			Boom  panicking `cbor:"2,keyasint"`
		}
		_ = cbor.Unmarshal(mustHex("a20161610200"), &values)
	}()
	for range 3 {
		calls = nil
		var value recorder
		if err := cbor.Unmarshal(mustHex("6162"), &value); err != nil || value.Text != "b" {
			t.Fatalf("decode after panic = %+v, %v", value, err)
		}
		if !slices.Equal(calls, []string{"b"}) {
			t.Fatalf("calls after panic = %q, want [b]", calls)
		}
	}
}

// TestDeferredMapEntries checks that maps whose elements are Unmarshalers
// gain no entries from an item that fails, and that a reused map is left
// as it was.
func TestDeferredMapEntries(t *testing.T) {
	calls = nil
	original := map[string]recorder{"old": {"x"}}
	value := original
	// {"a": "1", "b": "2", "a": "3"}: duplicate key after two entries.
	err := cbor.Unmarshal(mustHex("a3"+"61616131"+"61626132"+"61616133"), &value)
	if !errors.Is(err, cbor.ErrInvalid) || len(calls) != 0 {
		t.Fatalf("error = %v, calls %q", err, calls)
	}
	if len(value) != 1 || value["old"].Text != "x" {
		t.Fatalf("map changed: %v", value)
	}
	var pointers map[int]*recorder
	// {1: "1", 2: <reserved header>}
	if err := cbor.Unmarshal(mustHex("a2"+"016131"+"021c"), &pointers); !errors.Is(err, cbor.ErrMalformed) || len(calls) != 0 || pointers != nil {
		t.Fatalf("pointer map: %v, calls %q, map %v", err, calls, pointers)
	}
}

// accepting is an Unmarshaler that accepts any item and records the call.
type accepting struct{ Raw []byte }

func (value *accepting) UnmarshalCBOR(data []byte) error {
	calls = append(calls, "accepted")
	value.Raw = bytes.Clone(data)
	return nil
}

type deferralTarget struct {
	One    accepting            `cbor:"1,keyasint"`
	List   []accepting          `cbor:"2,keyasint"`
	Map    map[string]accepting `cbor:"3,keyasint"`
	Ints   map[int]*accepting   `cbor:"4,keyasint"`
	Tagged cbor.Tag[accepting]  `cbor:"5,keyasint"`
	Plain  []int                `cbor:"6,keyasint"`
	Any    any                  `cbor:"7,keyasint"`
}

// FuzzDeferral checks, for arbitrary input, that a failed decode ran no
// UnmarshalCBOR method and left the destination unchanged. accepting never
// fails, so every failure is the decoder's own.
func FuzzDeferral(f *testing.F) {
	for _, seed := range []string{
		"a50140" + "028140" + "03a1616b40" + "04a10140" + "05c140",
		"a3" + "0140" + "028140" + "0140",
		"a2" + "03a2616b40616b40" + "0601",
		"a1" + "07a2010201" + "03",
		"a1" + "0283404040",
	} {
		f.Add(mustHex(seed))
	}
	addSeeds(f)
	f.Fuzz(func(t *testing.T, data []byte) {
		for _, options := range []cbor.DecoderOptions{fuzzOptions, {RejectUnknownFields: true, RequireDeterministic: true}} {
			for _, original := range []deferralTarget{{}, {Plain: []int{1}, One: accepting{Raw: []byte{9}}}} {
				calls = nil
				value := original
				if err := options.Unmarshal(data, &value); err != nil {
					if len(calls) != 0 {
						t.Fatalf("%x: UnmarshalCBOR ran %d times before %v", data, len(calls), err)
					}
					if !reflect.DeepEqual(value, original) {
						t.Fatalf("%x: destination changed on %v", data, err)
					}
				}
			}
		}
	})
}
