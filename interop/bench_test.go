package interop

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	fx "github.com/fxamacker/cbor/v2"
	"github.com/picatz/cbor"
)

// Side-by-side benchmarks against the reference implementation, so
// performance claims in the main module are measured rather than asserted.
// Run with
//
//	go test -run '^$' -bench . -benchmem -count 10 | benchstat -col /impl -
//
// Both implementations decode with duplicate-key and UTF-8 checks enabled.

// cwtClaims is RFC 8392 Appendix A.1.
var cwtClaims, _ = hex.DecodeString("a70175636f61703a2f2f61732e6578616d706c652e636f6d02656572696b77037818636f61703a2f2f6c696768742e6578616d706c652e636f6d041a5612aeb0051a5610d9f0061a5610d9f007420b71")

type claims struct {
	Iss string `cbor:"1,keyasint"`
	Sub string `cbor:"2,keyasint"`
	Aud string `cbor:"3,keyasint"`
	Exp int    `cbor:"4,keyasint"`
	Nbf int    `cbor:"5,keyasint"`
	Iat int    `cbor:"6,keyasint"`
	Cti []byte `cbor:"7,keyasint"`
}

// coseKey is an RFC 9052 Section 7 EC2 key with integer labels.
type coseKey struct {
	Kty int    `cbor:"1,keyasint"`
	Kid []byte `cbor:"2,keyasint,omitempty"`
	Alg int    `cbor:"3,keyasint,omitempty"`
	Crv int    `cbor:"-1,keyasint"`
	X   []byte `cbor:"-2,keyasint"`
	Y   []byte `cbor:"-3,keyasint"`
}

// registration is shaped like a WebAuthn registration's text fields: a
// relying party and user entity with 15-60 byte strings, one non-ASCII.
type registration struct {
	RPID        string `cbor:"rpId"`
	RPName      string `cbor:"rpName"`
	UserName    string `cbor:"userName"`
	DisplayName string `cbor:"displayName"`
	Origin      string `cbor:"origin"`
}

var webauthn = registration{
	RPID:        "login.example.com",
	RPName:      "Example Corporation Sign-In Service",
	UserName:    "alice.johnson@example.com",
	DisplayName: "Alice Johnson — Platform Engineering",
	Origin:      "https://login.example.com/webauthn/register?flow=passkey",
}

// optional has omitzero fields, two of them time.Time, which reports zero
// through its IsZero method.
type optional struct {
	Issued  time.Time `cbor:"1,keyasint,omitzero"`
	Count   int       `cbor:"2,keyasint,omitzero"`
	Label   string    `cbor:"3,keyasint,omitzero"`
	Expires time.Time `cbor:"4,keyasint,omitzero"`
}

var key = coseKey{Kty: 2, Kid: []byte("11"), Alg: -7, Crv: 1, X: bytes.Repeat([]byte{0xba}, 32), Y: bytes.Repeat([]byte{0xbe}, 32)}

var getInfo = map[uint64]any{
	0x01: []any{"U2F_V2", "FIDO_2_0", "FIDO_2_1"},
	0x02: []any{"credProtect", "hmac-secret"},
	0x03: bytes.Repeat([]byte{0x2f}, 16),
	0x04: map[string]any{"rk": true, "up": true, "plat": false, "clientPin": true},
	0x05: uint64(1200),
	0x06: []any{uint64(2), uint64(1)},
	0x0a: []any{map[string]any{"alg": int64(-7), "type": "public-key"}, map[string]any{"alg": int64(-8), "type": "public-key"}},
}

var reference = func() fx.DecMode {
	mode, err := fx.DecOptions{DupMapKey: fx.DupMapKeyEnforcedAPF, UTF8: fx.UTF8RejectInvalid}.DecMode()
	if err != nil {
		panic(err)
	}
	return mode
}()

var referenceEncoder = func() fx.EncMode {
	mode, err := fx.CoreDetEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	return mode
}()

type codec struct {
	name      string
	unmarshal func([]byte, any) error
	marshal   func(any) ([]byte, error)
}

var codecs = []codec{
	{"impl=picatz", cbor.Unmarshal, cbor.CoreDeterministicEncoding().Marshal},
	{"impl=fxamacker", reference.Unmarshal, referenceEncoder.Marshal},
}

func BenchmarkUnmarshal(b *testing.B) {
	coseKeyData, err := cbor.Marshal(key)
	if err != nil {
		b.Fatal(err)
	}
	getInfoData, err := cbor.Marshal(getInfo)
	if err != nil {
		b.Fatal(err)
	}
	cases := []struct {
		name string
		data []byte
		new  func() any
	}{
		{"CWTClaims", cwtClaims, func() any { return new(claims) }},
		{"COSEKey", coseKeyData, func() any { return new(coseKey) }},
		{"GetInfoAny", getInfoData, func() any { return new(any) }},
		{"GetInfoRaw", getInfoData, func() any { return new(map[uint64]cbor.RawMessage) }},
	}
	for _, c := range cases {
		for _, impl := range codecs {
			if c.name == "GetInfoRaw" && impl.name != "impl=picatz" {
				continue
			}
			b.Run(c.name+"/"+impl.name, func(b *testing.B) {
				b.SetBytes(int64(len(c.data)))
				b.ReportAllocs()
				for b.Loop() {
					if err := impl.unmarshal(c.data, c.new()); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}

func BenchmarkMarshal(b *testing.B) {
	var decodedClaims claims
	if err := cbor.Unmarshal(cwtClaims, &decodedClaims); err != nil {
		b.Fatal(err)
	}
	cases := []struct {
		name  string
		value any
	}{
		{"CWTClaims", decodedClaims},
		{"COSEKey", key},
		{"GetInfoAny", getInfo},
		{"WebAuthnText", webauthn},
		{"OmitZero", &optional{Count: 3, Label: "x"}},
		{"MapStringBool", map[string]bool{"rk": true, "up": true, "uv": false, "plat": false, "clientPin": true}},
	}
	for _, c := range cases {
		for _, impl := range codecs {
			b.Run(c.name+"/"+impl.name, func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					if _, err := impl.marshal(c.value); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
