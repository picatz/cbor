package cbor_test

import (
	"bufio"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/picatz/cbor"
)

type coseVector struct {
	name string
	data []byte
	fail bool // intentionally invalid at the COSE level
}

// coseVectors reads the cose-wg/Examples corpus; see
// testdata/cose-wg/README.md.
func coseVectors(t testing.TB) []coseVector {
	t.Helper()
	file, err := os.Open("testdata/cose-wg/vectors.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	var vectors []coseVector
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 2 || len(fields) > 3 || len(fields) == 3 && fields[2] != "fail" {
			t.Fatalf("malformed line %q", scanner.Text())
		}
		vectors = append(vectors, coseVector{name: fields[0], data: mustHex(fields[1]), fail: len(fields) == 3})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(vectors) < 300 {
		t.Fatalf("read %d vectors, want the full corpus", len(vectors))
	}
	return vectors
}

// coseArity is the number of array elements of each tagged COSE message
// (RFC 9052 Section 2, Table 1).
var coseArity = map[uint64]int{
	16: 3, // COSE_Encrypt0
	17: 4, // COSE_Mac0
	18: 4, // COSE_Sign1
	96: 4, // COSE_Encrypt
	97: 5, // COSE_Mac
	98: 4, // COSE_Sign
}

// TestCOSEWorkingGroupExamples checks every cose-wg/Examples message, an
// independent corpus, for acceptance, structure and deterministic encoding.
func TestCOSEWorkingGroupExamples(t *testing.T) {
	strict := cbor.DecoderOptions{RequireDeterministic: true}
	deterministic, tagged := 0, 0
	for _, vector := range coseVectors(t) {
		t.Run(vector.name, func(t *testing.T) {
			if _, err := cbor.Diagnose(vector.data); err != nil {
				t.Fatalf("Diagnose: %v", err)
			}
			var decoded any
			if err := cbor.Unmarshal(vector.data, &decoded); err != nil {
				t.Fatal(err)
			}
			encoded, err := cbor.CoreDeterministicEncoding().Marshal(decoded)
			if err != nil {
				t.Fatal(err)
			}
			if err := strict.Unmarshal(encoded, new(any)); err != nil {
				t.Fatalf("deterministic re-encoding %x rejected: %v", encoded, err)
			}
			// Input already in deterministic form is a fixed point.
			if strict.Unmarshal(vector.data, new(cbor.RawMessage)) == nil {
				deterministic++
				if !bytes.Equal(encoded, vector.data) {
					t.Fatalf("deterministic input re-encoded as %x", encoded)
				}
			}

			var message cbor.Tag[[]cbor.RawMessage]
			if cbor.Unmarshal(vector.data, &message) != nil {
				return // untagged messages and keys
			}
			arity, ok := coseArity[message.Number]
			if !ok || vector.fail {
				return
			}
			tagged++
			if len(message.Content) != arity {
				t.Fatalf("tag %d has %d elements, want %d", message.Number, len(message.Content), arity)
			}
			// The protected header is a byte string holding a map, or empty.
			var protected []byte
			if err := cbor.Unmarshal(message.Content[0], &protected); err != nil {
				t.Fatalf("protected header: %v", err)
			}
			if len(protected) > 0 {
				if err := cbor.Unmarshal(protected, new(map[any]cbor.RawMessage)); err != nil {
					t.Fatalf("protected header map: %v", err)
				}
			}
			if err := cbor.Unmarshal(message.Content[1], new(map[any]cbor.RawMessage)); err != nil {
				t.Fatalf("unprotected header: %v", err)
			}
			// RawMessage keeps every element verbatim, so the message
			// re-encodes byte for byte.
			if again, err := cbor.Marshal(message); err != nil || !bytes.Equal(again, vector.data) {
				t.Fatalf("typed round trip = %x, %v", again, err)
			}
		})
	}
	if deterministic < 150 || tagged < 150 {
		t.Fatalf("only %d deterministic and %d tagged messages checked", deterministic, tagged)
	}
}

func FuzzCOSE(f *testing.F) {
	for _, vector := range coseVectors(f) {
		f.Add(vector.data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		var message cbor.Tag[[]cbor.RawMessage]
		if err := fuzzOptions.Unmarshal(data, &message); err != nil {
			return
		}
		again, err := cbor.Marshal(message)
		if err != nil {
			t.Fatalf("%x: re-encoding: %v", data, err)
		}
		var raw cbor.RawMessage
		if err := fuzzOptions.Unmarshal(again, &raw); err != nil {
			t.Fatalf("%x: re-encoding %x rejected: %v", data, again, err)
		}
	})
}
