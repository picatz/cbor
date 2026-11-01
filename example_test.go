package cbor_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"

	"github.com/picatz/cbor"
)

func ExampleMarshal() {
	type credentialParameter struct {
		Type      string `cbor:"type"`
		Algorithm int    `cbor:"alg"`
	}
	encoded, err := cbor.Marshal(credentialParameter{Type: "public-key", Algorithm: -7})
	if err != nil {
		panic(err)
	}
	fmt.Printf("%x\n", encoded)
	// Output: a263616c672664747970656a7075626c69632d6b6579
}

func ExampleUnmarshal() {
	// {1: "coap://as.example.com", 7: h'0b71'}, from RFC 8392 Appendix A.1.
	data := []byte("\xa2\x01\x75coap://as.example.com\x07\x42\x0b\x71")
	var claims struct {
		Issuer string `cbor:"1,keyasint"`
		ID     []byte `cbor:"7,keyasint"`
	}
	if err := cbor.Unmarshal(data, &claims); err != nil {
		panic(err)
	}
	fmt.Printf("%s %x\n", claims.Issuer, claims.ID)
	// Output: coap://as.example.com 0b71
}

func ExampleDecoderOptions_Unmarshal() {
	// Tight limits for an untrusted, FIDO-sized response.
	options := cbor.DecoderOptions{
		MaxEncodedBytes:  1024,
		MaxNestedLevels:  4,
		MaxArrayElements: 16,
		MaxMapPairs:      16,
		MaxStringBytes:   64,
		MaxBytes:         64,
		RejectTags:       true,
	}
	var value []uint64
	err := options.Unmarshal([]byte{0x98, 0x20}, &value) // an array claiming 32 elements
	fmt.Println(errors.Is(err, cbor.ErrLimit))
	// Output: true
}

func ExampleRawMessage() {
	// Decode the outer map, keeping each value encoded until its type is known.
	data := []byte("\xa2\x01\x82\x68FIDO_2_0\x68FIDO_2_1\x04\xa1\x62rk\xf5")
	var fields map[uint64]cbor.RawMessage
	if err := cbor.Unmarshal(data, &fields); err != nil {
		panic(err)
	}
	var versions []string
	if err := cbor.Unmarshal(fields[1], &versions); err != nil {
		panic(err)
	}
	fmt.Printf("%v %x\n", versions, []byte(fields[4]))
	// Output: [FIDO_2_0 FIDO_2_1] a162726bf5
}

func ExampleCoreDeterministicEncoding() {
	value := map[string]any{"b": 1.5, "a": []any{uint64(24), -1}}
	encoded, err := cbor.CoreDeterministicEncoding().Marshal(value)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%x\n", encoded)

	// RequireDeterministic accepts exactly this encoding.
	strict := cbor.DecoderOptions{RequireDeterministic: true}
	var decoded any
	fmt.Println(strict.Unmarshal(encoded, &decoded))
	fmt.Println(errors.Is(strict.Unmarshal([]byte{0xa2, 3, 4, 1, 2}, &decoded), cbor.ErrNotDeterministic))
	// Output:
	// a26161821818206162f93e00
	// <nil>
	// true
}

func ExampleDecoder_Decode() {
	// A CBOR sequence of three items, read one at a time.
	stream := bytes.NewReader([]byte{0x01, 0x62, 'h', 'i', 0xf5})
	decoder := cbor.NewDecoder(stream)
	for {
		var item any
		err := decoder.Decode(&item)
		if err == io.EOF {
			break
		}
		if err != nil {
			panic(err)
		}
		fmt.Printf("%v ", item)
	}
	fmt.Println()
	// Output: 1 hi true
}

func ExampleError() {
	err := cbor.Unmarshal([]byte{0xa2, 0x01, 0x02, 0x01, 0x03}, new(map[int]int))
	var detail *cbor.Error
	if errors.As(err, &detail) {
		fmt.Println(errors.Is(err, cbor.ErrInvalid), detail.Offset, detail.Msg)
	}
	// Output: true 3 duplicate map key
}

func ExampleDecoderOptions_rejectUnknownFields() {
	type header struct {
		Algorithm int    `cbor:"1,keyasint"`
		KeyID     []byte `cbor:"4,keyasint"`
	}
	strict := cbor.DecoderOptions{RejectUnknownFields: true, RequireDeterministic: true}

	// {1: -7, 4: h'6b', 5: true}: key 5 is not part of the header.
	var decoded header
	err := strict.Unmarshal([]byte{0xa3, 0x01, 0x26, 0x04, 0x41, 0x6b, 0x05, 0xf5}, &decoded)
	fmt.Println(errors.Is(err, cbor.ErrType), err)
	// Output: true cbor: unknown struct field at offset 6
}

func ExampleTag() {
	// A COSE_Sign1 message (RFC 9052 Section 4.2) is tag 18 around a
	// four-element array, so its struct uses toarray.
	type sign1 struct {
		_           struct{} `cbor:",toarray"`
		Protected   []byte
		Unprotected map[int64]cbor.RawMessage
		Payload     []byte
		Signature   []byte
	}
	message := cbor.Tag[sign1]{Number: 18, Content: sign1{
		Protected:   []byte{0xa1, 0x01, 0x26}, // {1: -7}, alg ES256
		Unprotected: map[int64]cbor.RawMessage{},
		Payload:     []byte("hi"),
		Signature:   []byte{0x01},
	}}
	encoded, err := cbor.CoreDeterministicEncoding().Marshal(message)
	if err != nil {
		panic(err)
	}
	fmt.Printf("%x\n", encoded)

	var decoded cbor.Tag[sign1]
	if err := cbor.Unmarshal(encoded, &decoded); err != nil {
		panic(err)
	}
	fmt.Println(decoded.Number, string(decoded.Content.Payload))
	// Output:
	// d28443a10126a04268694101
	// 18 hi
}

func ExampleDiagnose() {
	// A COSE header, {1: -7, 4: h'3131'}: alg ES256 and kid "11".
	notation, err := cbor.Diagnose([]byte{0xa2, 0x01, 0x26, 0x04, 0x42, 0x31, 0x31})
	if err != nil {
		panic(err)
	}
	fmt.Println(notation)
	// Output: {1: -7, 4: h'3131'}
}

func ExampleDecoder_All() {
	// An RFC 8742 CBOR sequence: 1, "hi", [true].
	stream := bytes.NewReader([]byte{0x01, 0x62, 'h', 'i', 0x81, 0xf5})
	for item, err := range cbor.NewDecoder(stream).All() {
		if err != nil {
			panic(err)
		}
		notation, _ := cbor.Diagnose(item)
		fmt.Println(notation)
	}
	// Output:
	// 1
	// "hi"
	// [true]
}

func ExampleAppendMarshal() {
	// Frame several items in one buffer, here after a self-described CBOR tag.
	buffer := []byte{0xd9, 0xd9, 0xf7}
	for _, value := range []any{1, "a"} {
		var err error
		if buffer, err = cbor.AppendMarshal(buffer, value); err != nil {
			panic(err)
		}
	}
	fmt.Printf("%x\n", buffer)
	// Output: d9d9f7016161
}

// Struct-wide integer keys and required fields describe a COSE_Key
// (RFC 9052 Section 7) strictly enough that no hand-written map walking is
// needed. Common parameters can live in an embedded struct.
func Example_integerKeys() {
	type keyCommon struct {
		_         struct{} `cbor:",keyasint"`
		KeyType   int64    `cbor:"1,required"`
		KeyID     []byte   `cbor:"2,omitempty"`
		Algorithm int64    `cbor:"3,omitempty"`
	}
	type ec2Key struct {
		keyCommon
		_     struct{} `cbor:",keyasint"`
		Curve int64    `cbor:"-1,required"`
		X     []byte   `cbor:"-2,required"`
	}
	strict := cbor.DecoderOptions{RejectUnknownFields: true, RequireDeterministic: true}

	// {1: 2, 3: -7, -1: 1, -2: h'01'}
	var key ec2Key
	err := strict.Unmarshal([]byte{0xa4, 0x01, 0x02, 0x03, 0x26, 0x20, 0x01, 0x21, 0x41, 0x01}, &key)
	fmt.Println(key.KeyType, key.Algorithm, key.Curve, key.X, err)

	// {1: 2, -2: h'01'}: the curve is missing.
	err = strict.Unmarshal([]byte{0xa2, 0x01, 0x02, 0x21, 0x41, 0x01}, &key)
	fmt.Println(err)
	// Output:
	// 2 -7 1 [1] <nil>
	// cbor: missing required field -1 (Curve) of cbor_test.ec2Key at offset 0
}

func Example_bigInt() {
	// 2^64 needs a bignum (tag 2); 2^64 - 1 fits an unsigned integer.
	for _, value := range []*big.Int{
		new(big.Int).Lsh(big.NewInt(1), 64),
		new(big.Int).SetUint64(math.MaxUint64),
	} {
		encoded, err := cbor.Marshal(value)
		if err != nil {
			panic(err)
		}
		notation, _ := cbor.Diagnose(encoded)
		fmt.Println(notation)
	}
	// Output:
	// 2(h'010000000000000000')
	// 18446744073709551615
}
