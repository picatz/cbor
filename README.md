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

Duplicate map keys, invalid UTF-8, reserved headers, indefinite lengths, integer
overflow and incompatible destination types are errors. Duplicate keys are
detected even with different integer/length encodings. Reused destination maps,
slices and structs are replaced on success and unchanged on an error, apart from
side effects inside application-provided `UnmarshalCBOR` methods. Unknown struct
fields are fully validated before being ignored. Struct metadata never retains
references to a previously decoded object.

`RawMessage` copies the complete original encoding, including nested values. It
supports delayed decoding and custom `Unmarshaler`/`Marshaler` implementations.
Raw and custom-marshaled values are validated before encoding.

## Supported values and compatibility

- Unsigned integers through `uint64`, negative integers through `int64`, booleans,
  null, UTF-8 text, byte strings, arrays, maps, and binary16/32/64 decoding.
- Integer-to-float decoding succeeds only when the integer is exactly
  representable. Float-to-integer coercion is not supported.
- Null decodes to nil pointers, interfaces, maps, and slices. Null into a scalar
  is an error. Undefined is preserved as `SimpleValueUndefined` in `any`.
- Tags are preserved as `TaggedValue`; application-specific date/URL/bignum
  interpretation is not performed. `RejectTags` disables tags altogether.
- Generic maps use `map[any]any`. Composite and NaN map keys are unsupported.
  Byte-string keys can be retained with `RawMessage`, but cannot become Go map
  keys. Embedded struct fields are not flattened.
- Exported struct fields support `cbor:"name"`, `keyasint`, `omitempty`, and `-`.
  Decoding prefers exact field names, then unambiguous case-insensitive matches.
- Encoding supports pointers, byte strings, negative integers, full unsigned
  ranges, struct key/value pairs, and deterministic map ordering. Encoders cap
  items at 1 MiB, nesting at 64 levels, and arrays/maps at 10,000 entries.
  Floats are encoded as binary64, so output is not fully canonical CBOR/CTAP.
- Encoders validate and buffer an item before writing it. A failed writer can
  still leave partial output; short writes return `io.ErrShortWrite`.

These corrections intentionally reject malformed data that older revisions
accepted. In particular, null pointers remain nil, malformed maps no longer
produce partial results, and unsafe semantic tag conversions are replaced by
explicit `TaggedValue` handling. The module continues to target Go 1.19.

## Validation

```sh
go test -race ./...
go vet ./...
go test -run '^$' -fuzz FuzzDecode -fuzztime 30s
```

Tests cover FIDO-shaped capability maps, RawMessage ownership, one-byte stream
reads, duplicate keys, truncation, limits, repeated and concurrent struct
decoding, numeric boundaries, encoder vectors, and malformed-input fuzzing.
