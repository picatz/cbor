# CBOR
 
[Concise Binary Object Representation](https://www.rfc-editor.org/rfc/rfc8949.html) (CBOR) is a schemaless data format 
designed to be small and extensible. This package implements a minimal CBOR encoder and decoder in a similar style to 
the [`encoding/json`](https://pkg.go.dev/encoding/json) package.

This package supports a deliberately bounded, definite-length CBOR subset.
It is not a complete RFC 8949 implementation or an independently audited codec.
Applications should review the limits and compatibility notes below before use.

```go
package main

import (
	"bytes"
	"fmt"

	"github.com/picatz/cbor"
)

// This code shows how to encode and decode the CBOR data format.
func main() {
	// CBOR encoding of the value {"hello": "world"}.
	const data = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64"

	// Create a new cbor.Decoder using bytes.NewBufferString(data) as its source,
	// and then Decode the CBOR data into the value map[string]string.
	var value map[string]string
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		panic(err)
	}

	// Output: world
	fmt.Println(value["hello"])

	// Encode the value map[string]string using the cbor.NewEncoder.
	//
    // Integer and length headers use their shortest form.
	var buf = bytes.NewBuffer(nil)
	err = cbor.NewEncoder(buf).Encode(value)
	if err != nil {
		panic(err)
	}

    // Output: a16568656c6c6f65776f726c64
	fmt.Printf("%x\n", buf.Bytes())
}
```

## Decoding untrusted input

`Unmarshal` accepts exactly one item and rejects trailing data. `Decoder.Decode`
reads one item without consuming the next, for CBOR streams. Truncation returns
`io.ErrUnexpectedEOF`; clean stream exhaustion returns `io.EOF`.

```go
options := cbor.DecoderOptions{
    MaxEncodedBytes: 7609,
    MaxNestedLevels: 16,
    MaxArrayElements: 256,
    MaxMapPairs: 128,
    MaxStringBytes: 7609,
    MaxBytes: 7609,
    RejectTags: true,
    RejectNonMinimal: true,
}
var fields map[uint64]cbor.RawMessage
err := options.Unmarshal(data, &fields)
```

Limits are checked before allocation or recursion. Zero limits select defaults:
10,000 array elements, map pairs, text bytes and byte-string bytes; 64 nesting
levels; and 1 MiB encoded input. The maximum configurable nesting limit is 256.
Decoder options are copied per instance; setters never modify another decoder.
Set the legacy `DefaultDecoderOptions` only before concurrent work starts.

Every failure other than I/O and end-of-input is a `*cbor.Error` with a byte
offset, classified for `errors.Is`:

| Kind | Meaning |
|------|---------|
| `ErrMalformed` | not well-formed: reserved headers, a stray break, `f8 00`–`f8 1f`, trailing data |
| `ErrInvalid` | well-formed but invalid: duplicate map keys, invalid UTF-8 |
| `ErrUnsupported` | valid but outside this profile: indefinite lengths, composite or NaN keys, integers below `math.MinInt64`, rejected tags |
| `ErrLimit` | over a configured limit |
| `ErrNotDeterministic` | rejected by `RejectNonMinimal` or `RequireDeterministic` |
| `ErrType` | valid, but not representable in the destination type |

Duplicate keys are detected in the CBOR data model, so `1` and `0x1801`, or
`1.0` as binary16 and binary64, collide. Reused destination maps, slices and
structs are replaced on success and unchanged on an error, apart from side
effects inside application-provided `UnmarshalCBOR` methods. Unknown struct
fields are fully validated before being ignored, or rejected with `ErrType`
when `RejectUnknownFields` is set; that option also turns off case-insensitive
field matching, so each key has exactly one accepted spelling. Struct metadata is cached per
type and never references a decoded or encoded value.

Decoded byte slices, strings and `RawMessage` values never alias the input.
`RawMessage` copies the complete original encoding, including nested values. It
supports delayed decoding and custom `Unmarshaler`/`Marshaler` implementations.
Raw and custom-marshaled values are validated before encoding.

## Deterministic encoding

Every encoder writes definite lengths and the shortest integer, length and tag
headers. `EncoderOptions` selects the rest:

| Options | Map key order | Floats |
|---------|---------------|--------|
| `EncoderOptions{}` (`Marshal`, `NewEncoder`) | length, then bytewise ([RFC 7049 §3.9](https://www.rfc-editor.org/rfc/rfc7049.html#section-3.9)) | binary64 |
| `CoreDeterministicEncoding()` | bytewise ([RFC 8949 §4.2.1](https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1)) | shortest exact width; NaN as `f97e00` |

Bytewise order of preferred-serialized keys is also the
[CTAP2 canonical](https://fidoalliance.org/specs/fido-v2.1-ps-20210615/fido-client-to-authenticator-protocol-v2.1-ps-errata-20220621.html#ctap2-canonical-cbor-encoding-form)
key order, so `EncoderOptions{Sort: cbor.SortBytewiseLexical}` produces CTAP2
key order while keeping binary64 floats. The default length-first order agrees
with it unless one map mixes key types whose lengths invert their major-type
order, such as `24` and `""`.

```go
encoded, err := cbor.CoreDeterministicEncoding().Marshal(value)

strict := cbor.DecoderOptions{RequireDeterministic: true}
err = strict.Unmarshal(encoded, &decoded) // ErrNotDeterministic for any other encoding
```

With `CoreDeterministicEncoding`, embedded `RawMessage` and `Marshaler` output
must already be deterministic, or encoding fails with `ErrNotDeterministic`.

## Supported values and compatibility

- Unsigned integers through `uint64`, negative integers through `int64`, booleans,
  null, undefined and other simple values, UTF-8 text, byte strings, arrays,
  maps, and binary16/32/64 floats.
- Integer-to-float decoding succeeds only when the integer is exactly
  representable. Float-to-integer coercion is not supported.
- Null decodes to nil pointers, interfaces, maps, and slices. Null into a scalar
  is an error. Undefined and unassigned simple values decode as `SimpleValue`.
- Tags are preserved as `TaggedValue`; application-specific date/URL/bignum
  interpretation and tag-content validation are not performed. `RejectTags`
  disables tags altogether.
- Generic maps use `map[any]any`. Composite and NaN map keys are unsupported.
  Byte-string keys can be retained with `RawMessage`, but cannot become Go map
  keys. Embedded struct fields are not flattened.
- `[]byte` and byte arrays such as `[16]byte` encode as byte strings. Byte
  strings decode into `[]byte` and into byte arrays of exactly matching length,
  such as a 16-byte AAGUID into `[16]byte`.
- Exported struct fields support `cbor:"name"`, `keyasint`, `omitempty`, and `-`.
  Decoding prefers exact field names, then unambiguous case-insensitive matches.
- Encoders cap items at 1 MiB, nesting at 64 levels, and arrays/maps at 10,000
  entries. Any value the default decoder produces re-encodes within the nesting
  limit; binary64 floats can make the re-encoding larger than its input.
- Encoders validate and buffer an item before writing it. A failed writer can
  still leave partial output; short writes return `io.ErrShortWrite`.

The module continues to target Go 1.19 and has no dependencies.

## Validation

```sh
go test -race ./...
go vet ./...
go test -run '^$' -fuzz FuzzDecode -fuzztime 30s       # also FuzzRoundTrip, FuzzStream, FuzzShortestFloat
env -i PATH="$PATH" HOME="$HOME" GOOS=js GOARCH=wasm go test -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./...
(cd interop && go test ./... && go test -run '^$' -fuzz FuzzDifferential -fuzztime 30s)
go test -run '^$' -bench . -benchmem
```

Conformance tests cover every RFC 8949 Appendix A example, including the
deterministic-encoding round trip the RFC marks for each. The separate
`interop` module differentially tests decoding, validity verdicts and
re-encoding against [fxamacker/cbor](https://github.com/fxamacker/cbor) and
compares core deterministic output byte for byte; it is a separate module so
this one keeps no dependencies. Fuzzers check decode/encode round trips,
deterministic fixed points, stream splitting with one-byte reads, truncation at
every offset, and shortest-float selection for arbitrary bit patterns. Unit
tests cover FIDO-shaped capability maps, ownership of decoded values, duplicate
keys, limits, error kinds and offsets, and concurrent struct decoding.

Benchmarks in `bench_test.go` use CTAP- and CWT-shaped payloads. Compare
revisions with [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).
