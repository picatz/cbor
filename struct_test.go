package cbor_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/picatz/cbor"
)

// coseSign1 is a COSE_Sign1 message (RFC 9052 Section 4.2): an array, not a
// map, so it uses toarray.
type coseSign1 struct {
	_           struct{} `cbor:",toarray"`
	Protected   []byte
	Unprotected map[int64]cbor.RawMessage
	Payload     []byte // nil for detached content
	Signature   []byte
}

// sigStructure is the Sig_structure a COSE_Sign1 signature covers (RFC 9052
// Section 4.4).
type sigStructure struct {
	_             struct{} `cbor:",toarray"`
	Context       string
	BodyProtected []byte
	ExternalAAD   []byte
	Payload       []byte
}

// TestCOSESign1Vector decodes RFC 9052 Appendix C.2.1 (identical to RFC 8152
// C.2.1), rebuilds the bytes to be signed, and verifies the ES256 signature.
// The vector, key and intermediate come from the public-domain
// https://github.com/cose-wg/Examples (RFC8152/Appendix_C_2_1.json).
func TestCOSESign1Vector(t *testing.T) {
	message, _ := hex.DecodeString("d28443a10126a10442313154546869732069732074686520636f6e74656e742e58408eb33e4ca31d1c465ab05aac34cc6b23d58fef5c083106c4d25a91aef0b0117e2af9a291aa32e14ab834dc56ed2a223444547e01f11d3b0916e5a4c345cacb36")
	wantToBeSigned := "846a5369676e61747572653143a101264054546869732069732074686520636f6e74656e742e"

	var tagged cbor.Tag[coseSign1]
	strict := cbor.DecoderOptions{RequireDeterministic: true, MaxNestedLevels: 4}
	if err := strict.Unmarshal(message, &tagged); err != nil {
		t.Fatal(err)
	}
	if tagged.Number != 18 {
		t.Fatalf("tag %d, want 18 (COSE_Sign1)", tagged.Number)
	}
	sign1 := tagged.Content
	var protected map[int64]int64
	if err := strict.Unmarshal(sign1.Protected, &protected); err != nil || protected[1] != -7 {
		t.Fatalf("protected header %v, %v; want alg ES256 (-7)", protected, err)
	}
	var kid []byte
	if err := cbor.Unmarshal(sign1.Unprotected[4], &kid); err != nil || string(kid) != "11" {
		t.Fatalf("kid %q, %v", kid, err)
	}

	toBeSigned, err := cbor.CoreDeterministicEncoding().Marshal(sigStructure{
		Context: "Signature1", BodyProtected: sign1.Protected, ExternalAAD: []byte{}, Payload: sign1.Payload,
	})
	if err != nil || hex.EncodeToString(toBeSigned) != wantToBeSigned {
		t.Fatalf("ToBeSigned = %x, %v; want %s", toBeSigned, err, wantToBeSigned)
	}
	coordinate := func(text string) *big.Int {
		data, err := base64.RawURLEncoding.DecodeString(text)
		if err != nil {
			t.Fatal(err)
		}
		return new(big.Int).SetBytes(data)
	}
	key := &ecdsa.PublicKey{Curve: elliptic.P256(),
		X: coordinate("usWxHK2PmfnHKwXPS54m0kTcGJ90UiglWiGahtagnv8"),
		Y: coordinate("IBOL-C3BttVivg-lSreASjpkttcsz-1rb7btKLv8EX4")}
	digest := sha256.Sum256(toBeSigned)
	r := new(big.Int).SetBytes(sign1.Signature[:32])
	s := new(big.Int).SetBytes(sign1.Signature[32:])
	if !ecdsa.Verify(key, digest[:], r, s) {
		t.Fatal("signature does not verify")
	}

	// Re-encoding the decoded message reproduces it byte for byte.
	again, err := cbor.CoreDeterministicEncoding().Marshal(tagged)
	if err != nil || !bytes.Equal(again, message) {
		t.Fatalf("re-encoded %x, %v", again, err)
	}
}

func TestToArray(t *testing.T) {
	type point struct {
		_ struct{} `cbor:",toarray"`
		X int64
		Y int64
		Z int64 `cbor:"-"`
	}
	encoded, err := cbor.Marshal(point{X: 1, Y: -2, Z: 9})
	if err != nil || hex.EncodeToString(encoded) != "820121" {
		t.Fatalf("%x, %v", encoded, err)
	}
	var decoded point
	if err := cbor.Unmarshal(encoded, &decoded); err != nil || decoded.X != 1 || decoded.Y != -2 {
		t.Fatal(decoded, err)
	}
	for _, input := range []string{"8101", "83010203", "a201020304", "01"} {
		data, _ := hex.DecodeString(input)
		if err := cbor.Unmarshal(data, &decoded); !errors.Is(err, cbor.ErrType) {
			t.Errorf("%s: %v, want ErrType", input, err)
		}
	}
	// A map-encoded struct rejects arrays.
	var plain struct{ X int64 }
	if err := cbor.Unmarshal([]byte{0x81, 0x01}, &plain); !errors.Is(err, cbor.ErrType) {
		t.Errorf("array into map struct: %v", err)
	}
	type omitting struct {
		_ struct{} `cbor:",toarray"`
		X int      `cbor:",omitempty"`
	}
	if _, err := cbor.Marshal(omitting{}); err == nil || !strings.Contains(err.Error(), "toarray") {
		t.Errorf("toarray with omitempty: %v", err)
	}
	type misplaced struct {
		X int `cbor:",toarray"`
	}
	if _, err := cbor.Marshal(misplaced{}); err == nil {
		t.Error("accepted toarray on a named field")
	}
}

type zeroer struct{ set bool }

func (value zeroer) IsZero() bool { return !value.set }

func TestOmitZero(t *testing.T) {
	type record struct {
		Count   int       `cbor:"1,keyasint,omitzero"`
		Empty   []int     `cbor:"2,keyasint,omitzero"` // a non-nil empty slice is not zero
		When    time.Time `cbor:"3,keyasint,omitzero"`
		Custom  zeroer    `cbor:"4,keyasint,omitzero"`
		Pointer *zeroer   `cbor:"5,keyasint,omitzero"`
		Array   [2]int    `cbor:"6,keyasint,omitzero"`
	}
	encoded, err := cbor.Marshal(record{Empty: []int{}})
	if err != nil || hex.EncodeToString(encoded) != "a10280" {
		t.Fatalf("%x, %v", encoded, err)
	}
	encoded, err = cbor.Marshal(record{Count: 1, Custom: zeroer{set: true}, Array: [2]int{0, 1}})
	if err != nil || hex.EncodeToString(encoded) != "a3010104a006820001" {
		t.Fatalf("%x, %v", encoded, err)
	}
}

func TestTagGeneric(t *testing.T) {
	encoded, err := cbor.Marshal(cbor.Tag[uint64]{Number: cbor.TagEpochTime, Content: 1363896240})
	if err != nil || hex.EncodeToString(encoded) != "c11a514b67b0" {
		t.Fatalf("%x, %v", encoded, err)
	}
	var epoch cbor.Tag[int64]
	if err := cbor.Unmarshal(encoded, &epoch); err != nil || epoch.Number != 1 || epoch.Content != 1363896240 {
		t.Fatal(epoch, err)
	}
	if err := cbor.Unmarshal([]byte{0x01}, &epoch); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("untagged item: %v", err)
	}
	if err := cbor.Unmarshal([]byte{0xc1, 0x61, 'a'}, &epoch); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("wrong content type: %v", err)
	}
	// Tags nest, and pointers to Tag work as fields.
	type wrapper struct {
		Inner *cbor.Tag[cbor.Tag[string]] `cbor:"1,keyasint"`
	}
	value := wrapper{Inner: &cbor.Tag[cbor.Tag[string]]{Number: 24, Content: cbor.Tag[string]{Number: 32, Content: "x"}}}
	encoded, err = cbor.Marshal(value)
	if err != nil || hex.EncodeToString(encoded) != "a101d818d8206178" {
		t.Fatalf("%x, %v", encoded, err)
	}
	var decoded wrapper
	if err := cbor.Unmarshal(encoded, &decoded); err != nil || decoded.Inner.Content.Content != "x" || decoded.Inner.Content.Number != 32 {
		t.Fatal(decoded, err)
	}
}
