package cbor_test

import (
	"bytes"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/picatz/cbor"
)

// claimsSet is the RFC 8392 claims set with struct-wide integer keys.
//
// https://www.rfc-editor.org/rfc/rfc8392.html#section-3.1
type claimsSet struct {
	_          struct{} `cbor:",keyasint"`
	Issuer     string   `cbor:"1,omitempty"`
	Subject    string   `cbor:"2,omitempty"`
	Audience   string   `cbor:"3,omitempty"`
	Expiration int64    `cbor:"4,required"`
	NotBefore  int64    `cbor:"5,omitempty"`
	IssuedAt   int64    `cbor:"6,omitempty"`
	CWTID      []byte   `cbor:"7,omitempty"`
}

// ec2Key is an RFC 9052 Section 7 COSE_Key with RFC 9053 EC2 parameters.
// The common parameters live in an embedded struct, as a shared header would.
type coseKeyCommon struct {
	_         struct{} `cbor:",keyasint"`
	KeyType   int64    `cbor:"1,required"`
	KeyID     []byte   `cbor:"2,omitempty"`
	Algorithm int64    `cbor:"3,omitempty"`
}

type ec2Key struct {
	coseKeyCommon
	_     struct{} `cbor:",keyasint"`
	Curve int64    `cbor:"-1,required"`
	X     []byte   `cbor:"-2,required"`
	Y     []byte   `cbor:"-3"`
}

// TestCWTClaimsStrict decodes RFC 8392 Appendix A.1 with every strictness
// option, which together replace re-encoding a message to check its shape.
func TestCWTClaimsStrict(t *testing.T) {
	data := mustHex("a70175636f61703a2f2f61732e6578616d706c652e636f6d02656572696b77037818636f61703a2f2f6c696768742e6578616d706c652e636f6d041a5612aeb0051a5610d9f0061a5610d9f007420b71")
	strict := cbor.DecoderOptions{RejectUnknownFields: true, RequireDeterministic: true}
	var claims claimsSet
	if err := strict.Unmarshal(data, &claims); err != nil {
		t.Fatal(err)
	}
	want := claimsSet{Issuer: "coap://as.example.com", Subject: "erikw", Audience: "coap://light.example.com",
		Expiration: 1444064944, NotBefore: 1443944944, IssuedAt: 1443944944, CWTID: []byte{0x0b, 0x71}}
	if claims.Issuer != want.Issuer || claims.Subject != want.Subject || claims.Audience != want.Audience ||
		claims.Expiration != want.Expiration || claims.NotBefore != want.NotBefore ||
		claims.IssuedAt != want.IssuedAt || !bytes.Equal(claims.CWTID, want.CWTID) {
		t.Fatalf("decoded %+v, want %+v", claims, want)
	}
	encoded, err := cbor.CoreDeterministicEncoding().Marshal(claims)
	if err != nil || !bytes.Equal(encoded, data) {
		t.Fatalf("re-encoded %x, %v; want %x", encoded, err, data)
	}

	for _, test := range []struct {
		name, encoded string
		kind          error
	}{
		{"missing required exp", "a10175636f61703a2f2f61732e6578616d706c652e636f6d", cbor.ErrType},
		{"unknown claim 8", "a2041a5612aeb008f5", cbor.ErrType},
		{"text key for integer field", "a1636578701a5612aeb0", cbor.ErrType},
		{"duplicate exp", "a2041a5612aeb0041a5612aeb1", cbor.ErrInvalid},
		{"non-minimal key", "a118041a5612aeb0", cbor.ErrNotDeterministic},
		{"keys out of order", "a2061a5610d9f0041a5612aeb0", cbor.ErrNotDeterministic},
		{"wrong type for exp", "a1046474657374", cbor.ErrType},
		{"array instead of map", "8104", cbor.ErrType},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := claimsSet{Issuer: "unchanged"}
			claims := before
			err := strict.Unmarshal(mustHex(test.encoded), &claims)
			if !errors.Is(err, test.kind) {
				t.Fatalf("error = %v, want %v", err, test.kind)
			}
			if claims.Issuer != before.Issuer || claims.Expiration != 0 {
				t.Fatalf("destination changed on error: %+v", claims)
			}
		})
	}
}

func TestRequiredFieldMessage(t *testing.T) {
	var claims claimsSet
	err := cbor.Unmarshal(mustHex("a0"), &claims)
	var detail *cbor.Error
	if !errors.As(err, &detail) || !errors.Is(err, cbor.ErrType) || detail.Offset != 0 ||
		!strings.Contains(detail.Msg, "missing required field 4 (Expiration)") {
		t.Fatalf("error = %v", err)
	}
	// A zero destination is decoded in place; a late failure resets it.
	claims = claimsSet{}
	if err := cbor.Unmarshal(mustHex("a20175636f61703a2f2f61732e6578616d706c652e636f6d0563626164"), &claims); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("error = %v", err)
	}
	if claims.Issuer != "" {
		t.Fatalf("destination changed on error: %+v", claims)
	}
	// A zero value still counts as present when its key is present.
	if err := cbor.Unmarshal(mustHex("a10400"), &claims); err != nil {
		t.Fatal(err)
	}
}

// TestCOSEKeyEmbedded decodes an EC2 key whose common parameters come from an
// embedded struct, and checks negative labels and required fields from both
// levels.
func TestCOSEKeyEmbedded(t *testing.T) {
	x := bytes.Repeat([]byte{0xba}, 32)
	key := ec2Key{coseKeyCommon: coseKeyCommon{KeyType: 2, KeyID: []byte("11"), Algorithm: -7}, Curve: 1, X: x, Y: x}
	encoded, err := cbor.CoreDeterministicEncoding().Marshal(key)
	if err != nil {
		t.Fatal(err)
	}
	// {1: 2, 2: h'3131', 3: -7, -1: 1, -2: h'ba..', -3: h'ba..'}
	want := "a6010202423131032620012158 20" + strings.Repeat("ba", 32) + "2258 20" + strings.Repeat("ba", 32)
	if got := hex.EncodeToString(encoded); got != strings.ReplaceAll(want, " ", "") {
		t.Fatalf("encoded %s", got)
	}
	strict := cbor.DecoderOptions{RejectUnknownFields: true, RequireDeterministic: true}
	var decoded ec2Key
	if err := strict.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.KeyType != 2 || decoded.Algorithm != -7 || string(decoded.KeyID) != "11" || decoded.Curve != 1 || !bytes.Equal(decoded.X, x) {
		t.Fatalf("decoded %+v", decoded)
	}
	// {-1: 1, -2: h'00'}: kty, required by the embedded struct, is missing.
	if err := strict.Unmarshal(mustHex("a22001214100"), &decoded); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("missing kty: %v", err)
	}
}

type Base struct {
	ID   string `cbor:"id"`
	Kind string `cbor:"kind"`
}

type Audit struct {
	Created int64 `cbor:"created"`
}

type withPointer struct {
	*Audit
	Base
	Kind string `cbor:"kind"` // shallower, so it dominates Base.Kind
}

func TestEmbeddedFields(t *testing.T) {
	value := withPointer{Base: Base{ID: "a", Kind: "hidden"}, Kind: "outer"}
	encoded, err := cbor.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	// A nil embedded pointer contributes no fields.
	if notation, _ := cbor.Diagnose(encoded); notation != `{"id": "a", "kind": "outer"}` {
		t.Fatalf("encoded %s", notation)
	}
	var decoded withPointer
	if err := cbor.Unmarshal(mustHex("a36269646161646b696e6465696e6e65726763726561746564182a"), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Audit == nil || decoded.Created != 42 || decoded.ID != "a" || decoded.Kind != "inner" || decoded.Base.Kind != "" {
		t.Fatalf("decoded %+v (audit %+v)", decoded, decoded.Audit)
	}
	// Absent promoted fields leave the embedded pointer nil.
	decoded = withPointer{}
	if err := cbor.Unmarshal(mustHex("a16269646161"), &decoded); err != nil || decoded.Audit != nil {
		t.Fatalf("decoded %+v, %v", decoded, err)
	}
}

type unexportedEmbedded struct {
	base        // fields promoted from an unexported struct type
	Name string `cbor:"name"`
}

type base struct {
	Version int `cbor:"v"`
}

func TestUnexportedEmbeddedStruct(t *testing.T) {
	encoded, err := cbor.Marshal(unexportedEmbedded{base: base{Version: 2}, Name: "n"})
	if err != nil {
		t.Fatal(err)
	}
	var decoded unexportedEmbedded
	if err := cbor.Unmarshal(encoded, &decoded); err != nil || decoded.Version != 2 || decoded.Name != "n" {
		t.Fatalf("decoded %+v, %v", decoded, err)
	}
}

func TestEmbeddedToArray(t *testing.T) {
	type header struct {
		Algorithm int64 `cbor:"1"`
	}
	type message struct {
		_ struct{} `cbor:",toarray"`
		header
		Payload []byte
	}
	encoded, err := cbor.Marshal(message{header: header{Algorithm: -7}, Payload: []byte{1}})
	if err != nil || hex.EncodeToString(encoded) != "82264101" {
		t.Fatalf("encoded %x, %v", encoded, err)
	}
	var decoded message
	if err := cbor.Unmarshal(encoded, &decoded); err != nil || decoded.Algorithm != -7 {
		t.Fatalf("decoded %+v, %v", decoded, err)
	}
}

// TestInvalidSchemas checks that ambiguous or contradictory struct
// definitions fail on first use instead of silently dropping fields.
func TestInvalidSchemas(t *testing.T) {
	type left struct {
		Name string `cbor:"name"`
	}
	type right struct {
		Label string `cbor:"name"`
	}
	type pointerArray struct {
		_ struct{} `cbor:",toarray"`
		*Base
	}
	type flattenedArray struct {
		_ struct{} `cbor:",toarray"`
		A int
	}
	for name, value := range map[string]any{
		"same key at same depth": struct {
			left
			right
		}{},
		"same key in one struct": struct {
			A int `cbor:"1,keyasint"`
			B int `cbor:"1,keyasint"`
		}{},
		"tagged and untagged at one depth": struct {
			A int
			B int `cbor:"A"`
		}{},
		"keyasint without integer name": struct {
			_ struct{} `cbor:",keyasint"`
			A int
		}{},
		"keyasint with text name": struct {
			_ struct{} `cbor:",keyasint"`
			A int      `cbor:"alg"`
		}{},
		"plus-signed key": struct {
			A int `cbor:"+1,keyasint"`
		}{},
		"required and omitempty": struct {
			A int `cbor:"a,required,omitempty"`
		}{},
		"toarray with keyasint": struct {
			_ struct{} `cbor:",toarray,keyasint"`
			A int
		}{},
		"unknown blank option": struct {
			_ struct{} `cbor:",sorted"`
		}{},
		"unknown field option": struct {
			A int `cbor:"a,inline"`
		}{},
		"toarray embeds pointer": pointerArray{},
		"embedded toarray": struct {
			flattenedArray
		}{},
		"options on embedded struct": struct {
			left `cbor:",omitempty"`
		}{},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := cbor.Marshal(value); !errors.Is(err, cbor.ErrUnsupported) {
				t.Errorf("Marshal error = %v, want ErrUnsupported", err)
			}
		})
	}
}

// TestKeyAsIntEquivalence checks that a struct-wide keyasint struct and its
// per-field spelling encode identically.
func TestKeyAsIntEquivalence(t *testing.T) {
	type wide struct {
		_   struct{} `cbor:",keyasint"`
		Alg int      `cbor:"3"`
		Kty int      `cbor:"1"`
		Crv int      `cbor:"-1"`
	}
	type perField struct {
		Alg int `cbor:"3,keyasint"`
		Kty int `cbor:"1,keyasint"`
		Crv int `cbor:"-1,keyasint"`
	}
	left, err := cbor.Marshal(wide{Alg: -7, Kty: 2, Crv: 1})
	if err != nil {
		t.Fatal(err)
	}
	right, err := cbor.Marshal(perField{Alg: -7, Kty: 2, Crv: 1})
	if err != nil || !bytes.Equal(left, right) {
		t.Fatalf("%x != %x (%v)", left, right, err)
	}
}

func TestStructWithManyFields(t *testing.T) {
	type many struct {
		_                                                struct{} `cbor:",keyasint"`
		F0, F1, F2, F3, F4, F5, F6, F7, F8, F9, F10, F11 int      `cbor:"0"`
	}
	// Struct tags apply to all names in a list, so build the type through
	// distinct keys instead.
	type wide struct {
		A0  int `cbor:"0,keyasint"`
		A1  int `cbor:"1,keyasint"`
		A2  int `cbor:"2,keyasint"`
		A3  int `cbor:"3,keyasint"`
		A4  int `cbor:"4,keyasint"`
		A5  int `cbor:"5,keyasint"`
		A6  int `cbor:"6,keyasint"`
		A7  int `cbor:"7,keyasint"`
		A8  int `cbor:"8,keyasint"`
		A9  int `cbor:"9,keyasint"`
		A10 int `cbor:"-1,keyasint"`
		A11 int `cbor:"-2,keyasint"`
		Txt int `cbor:"text"`
		Req int `cbor:"req,required"`
	}
	if _, err := cbor.Marshal(many{}); !errors.Is(err, cbor.ErrUnsupported) {
		t.Fatalf("duplicate keys from one tag: %v", err)
	}
	value := wide{A0: 1, A5: 5, A9: 9, A10: -1, A11: -2, Txt: 7, Req: 3}
	encoded, err := cbor.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded wide
	if err := (cbor.DecoderOptions{RejectUnknownFields: true}).Unmarshal(encoded, &decoded); err != nil || decoded != value {
		t.Fatalf("decoded %+v, %v", decoded, err)
	}
	// Case-insensitive matching still works on the hash-table path.
	if err := cbor.Unmarshal(mustHex("a26454455854076352455103"), &decoded); err != nil || decoded.Txt != 7 || decoded.Req != 3 {
		t.Fatalf("decoded %+v, %v", decoded, err)
	}
	if err := cbor.Unmarshal(mustHex("a1182a00"), &decoded); !errors.Is(err, cbor.ErrType) {
		t.Fatalf("missing required field: %v", err)
	}
}
