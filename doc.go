// Package cbor implements a bounded Concise Binary Object Representation
// (CBOR) encoder and decoder in the style of encoding/json.
//
// # Profile
//
// The package implements the definite-length subset of RFC 8949. Decoding
// accepts every well-formed, valid item except:
//
//   - indefinite-length strings, arrays and maps;
//   - negative integers below math.MinInt64;
//   - array, map, tag and NaN map keys;
//   - items over the DecoderOptions resource limits.
//
// Those items fail with ErrUnsupported or ErrLimit. Items that are not
// well-formed fail with ErrMalformed and invalid items, such as maps with
// duplicate keys (compared in the CBOR data model, so 1 and 0x1801 collide)
// or text with invalid UTF-8, fail with ErrInvalid. Tags are preserved as
// TaggedValue without interpreting or validating their content; simple values
// other than false, true, null and the floats decode as SimpleValue.
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
