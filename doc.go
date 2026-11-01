// Package cbor implements a bounded Concise Binary Object Representation
// (CBOR) encoder and decoder in the style of encoding/json, for security
// protocols such as COSE, CWT, WebAuthn and CTAP2.
//
// # Profile
//
// The package implements the definite-length subset of RFC 8949. Decoding
// accepts every well-formed, valid item except:
//
//   - indefinite-length strings, arrays and maps;
//   - array, map, tag and NaN map keys;
//   - items over the DecoderOptions resource limits.
//
// Those items fail with ErrUnsupported or ErrLimit. Items that are not
// well-formed fail with ErrMalformed and invalid items, such as maps with
// duplicate keys (compared in the CBOR data model, so 1 and 0x1801 collide)
// or text with invalid UTF-8, fail with ErrInvalid. The whole item is
// validated before any of it is decoded. Tags are preserved as TaggedValue or
// Tag without interpreting their content, except that bignums (tags 2 and 3)
// decode into big.Int; simple values other than false, true, null and the
// floats decode as SimpleValue.
//
// # Struct tags
//
// Struct fields map to text keys by default. The tag `cbor:"-7,keyasint"`
// selects integer key -7, and a blank field tagged `cbor:",keyasint"` makes
// every field's name an integer key, as COSE and CWT use:
//
//	type Claims struct {
//		_          struct{} `cbor:",keyasint"`
//		Issuer     string   `cbor:"1,omitempty"`
//		Expiration int64    `cbor:"4,required"`
//	}
//
// A blank field tagged `cbor:",toarray"` encodes the struct as an array of
// its fields. The options omitempty and omitzero omit fields when encoding,
// and required fails decoding with ErrType when a field's key is absent.
// Fields of embedded structs are promoted as in encoding/json, except that
// two fields with one key at the same depth are an error.
//
// # Encoding contracts
//
// Every encoder writes definite lengths and the shortest integer, length and
// tag headers. EncoderOptions then selects the map key order and the float
// widths. The default orders keys length-first (RFC 7049 canonical) and writes
// binary64 floats. CoreDeterministicEncoding produces RFC 8949 core
// deterministic encoding, whose key order is also CTAP2 canonical order, and
// DecoderOptions.RequireDeterministic accepts exactly that encoding.
//
// # Ownership
//
// Decoded byte slices, strings and RawMessage values never alias the input.
// Unmarshaler implementations receive a slice that may alias it and must copy
// what they retain.
//
// https://www.rfc-editor.org/rfc/rfc8949.html
package cbor
