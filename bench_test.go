package cbor_test

import (
	"bytes"
	"encoding/hex"
	"strconv"
	"testing"

	"github.com/picatz/cbor"
)

// Representative workloads for allocation and throughput measurements. Run with
//
//	go test -run '^$' -bench . -benchmem -count 10 > new.txt
//
// and compare revisions with golang.org/x/perf/cmd/benchstat.

// authenticatorGetInfo is a CTAP 2.1 authenticatorGetInfo response body shaped
// like a current security key: versions, extensions, AAGUID, options, maximum
// message size, PIN/UV protocols, transports and algorithms.
var authenticatorGetInfo = mustMarshal(map[uint64]any{
	0x01: []string{"U2F_V2", "FIDO_2_0", "FIDO_2_1", "FIDO_2_1_PRE"},
	0x02: []string{"credBlob", "credProtect", "hmac-secret", "largeBlobKey", "minPinLength"},
	0x03: bytes.Repeat([]byte{0x2f}, 16),
	0x04: map[string]bool{"rk": true, "up": true, "plat": false, "clientPin": true, "credMgmt": true, "largeBlobs": true, "pinUvAuthToken": true, "setMinPINLength": true},
	0x05: 1200,
	0x06: []uint64{2, 1},
	0x07: 8,
	0x08: 128,
	0x09: []string{"usb", "nfc"},
	0x0a: []map[string]any{{"alg": -7, "type": "public-key"}, {"alg": -8, "type": "public-key"}},
	0x0b: 1024,
})

// cwtClaims is RFC 8392 Appendix A.1.
var cwtClaims = mustHex("a70175636f61703a2f2f61732e6578616d706c652e636f6d02656572696b77037818636f61703a2f2f6c696768742e6578616d706c652e636f6d041a5612aeb0051a5610d9f0061a5610d9f007420b71")

type benchmarkClaims struct {
	Iss string `cbor:"1,keyasint"`
	Sub string `cbor:"2,keyasint"`
	Aud string `cbor:"3,keyasint"`
	Exp int    `cbor:"4,keyasint"`
	Nbf int    `cbor:"5,keyasint"`
	Iat int    `cbor:"6,keyasint"`
	Cti []byte `cbor:"7,keyasint"`
}

// makeCredential mirrors the shape of a CTAP authenticatorMakeCredential
// request: integer keys, nested text maps, and a list of algorithm entries.
type makeCredential struct {
	ClientDataHash   []byte            `cbor:"1,keyasint"`
	RP               map[string]string `cbor:"2,keyasint"`
	User             map[string]any    `cbor:"3,keyasint"`
	PubKeyCredParams []map[string]any  `cbor:"4,keyasint"`
	Options          map[string]bool   `cbor:"7,keyasint,omitempty"`
	PinUvAuthParam   []byte            `cbor:"8,keyasint,omitempty"`
	PinUvAuthProto   uint64            `cbor:"9,keyasint,omitempty"`
}

var makeCredentialRequest = makeCredential{
	ClientDataHash: bytes.Repeat([]byte{0xab}, 32),
	RP:             map[string]string{"id": "example.com", "name": "Example"},
	User:           map[string]any{"id": bytes.Repeat([]byte{1}, 16), "name": "user@example.com", "displayName": "User"},
	PubKeyCredParams: []map[string]any{
		{"alg": -7, "type": "public-key"},
		{"alg": -8, "type": "public-key"},
	},
	Options:        map[string]bool{"rk": true},
	PinUvAuthParam: bytes.Repeat([]byte{2}, 32),
	PinUvAuthProto: 2,
}

func mustMarshal(value any) []byte {
	data, err := cbor.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func mustHex(encoded string) []byte {
	data, err := hex.DecodeString(encoded)
	if err != nil {
		panic(err)
	}
	return data
}

func BenchmarkUnmarshalGetInfoRaw(b *testing.B) {
	b.SetBytes(int64(len(authenticatorGetInfo)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var fields map[uint64]cbor.RawMessage
		if err := cbor.Unmarshal(authenticatorGetInfo, &fields); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalGetInfoAny(b *testing.B) {
	b.SetBytes(int64(len(authenticatorGetInfo)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var value any
		if err := cbor.Unmarshal(authenticatorGetInfo, &value); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalStruct(b *testing.B) {
	b.SetBytes(int64(len(cwtClaims)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		var claims benchmarkClaims
		if err := cbor.Unmarshal(cwtClaims, &claims); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalByteString(b *testing.B) {
	for _, size := range []int{64, 1024, 7609} {
		payload, err := cbor.Marshal(bytes.Repeat([]byte{0x5a}, size))
		if err != nil {
			b.Fatal(err)
		}
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			b.SetBytes(int64(len(payload)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				var value []byte
				if err := cbor.Unmarshal(payload, &value); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDecoderStream(b *testing.B) {
	var stream []byte
	for index := 0; index < 64; index++ {
		stream = append(stream, cwtClaims...)
	}
	b.SetBytes(int64(len(stream)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		decoder := cbor.NewDecoder(bytes.NewReader(stream))
		for index := 0; index < 64; index++ {
			var claims benchmarkClaims
			if err := decoder.Decode(&claims); err != nil {
				b.Fatal(err)
			}
		}
	}
}

func BenchmarkMarshalStruct(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(makeCredentialRequest); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalMap(b *testing.B) {
	value := map[string]bool{"rk": true, "up": true, "uv": false, "plat": false, "clientPin": true, "credMgmt": true}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := cbor.Marshal(value); err != nil {
			b.Fatal(err)
		}
	}
}
