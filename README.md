# CBOR

[Concise Binary Object Representation](https://www.rfc-editor.org/rfc/rfc8949.html)
(CBOR) for Go, in the style of [`encoding/json`](https://pkg.go.dev/encoding/json),
built for security protocols such as COSE, CWT, WebAuthn and CTAP2:

- **Bounded and strict.** Every limit is checked before allocation or
  recursion. Duplicate keys, invalid UTF-8 and malformed items are always
  rejected, with typed errors and byte offsets.
- **Deterministic.** RFC 8949 core deterministic encoding, and a decoder that
  accepts exactly that encoding.
- **Schemas in struct tags.** Integer keys, required fields, unknown-field
  rejection, `toarray` structs and typed tags describe COSE and CWT messages
  without hand-written map walking.
- **Fast.** At parity with or faster than
  [fxamacker/cbor](https://github.com/fxamacker/cbor) v2.10 on most COSE,
  CWT and CTAP2 workloads, with the same allocations and without `unsafe`;
  [see below](#performance).
- **No dependencies**, and differentially tested against fxamacker/cbor.

This package supports a deliberately bounded, definite-length CBOR subset.
It is not an independently audited codec. Review the limits and
compatibility notes below before use.

```sh
go get github.com/picatz/cbor
```

```go
// CWT claims (RFC 8392) with integer keys and a required expiration.
type Claims struct {
	_          struct{} `cbor:",keyasint"`
	Issuer     string   `cbor:"1,omitempty"`
	Subject    string   `cbor:"2,omitempty"`
	Expiration int64    `cbor:"4,required"`
}

encoded, err := cbor.CoreDeterministicEncoding().Marshal(Claims{Issuer: "as.example", Expiration: 1444064944})

strict := cbor.DecoderOptions{RejectUnknownFields: true, RequireDeterministic: true}
var claims Claims
err = strict.Unmarshal(encoded, &claims)

notation, err := cbor.Diagnose(encoded) // {1: "as.example", 4: 1444064944}
```

## Decoding untrusted input

`Unmarshal` accepts exactly one item and rejects trailing data. `Decoder.Decode`
reads one item without consuming the next, and `Decoder.All` iterates a
[CBOR sequence](https://www.rfc-editor.org/rfc/rfc8742.html). Truncation
returns `io.ErrUnexpectedEOF`; clean stream exhaustion returns `io.EOF`.

`DecoderOptions` is a plain value, like protobuf's `UnmarshalOptions`: the zero
value selects the defaults, and its methods decode with it.

```go
options := cbor.DecoderOptions{
	MaxEncodedBytes:  7609,
	MaxNestedLevels:  16,
	MaxArrayElements: 256,
	MaxMapPairs:      128,
	MaxStringBytes:   7609,
	MaxBytes:         7609,
	RejectTags:       true,
	RejectNonMinimal: true,
}
var fields map[uint64]cbor.RawMessage
err := options.Unmarshal(data, &fields)
```

Zero limits select defaults: 10,000 array elements, map pairs, text bytes and
byte-string bytes; 64 nesting levels; and 1 MiB encoded input. The maximum
configurable nesting limit is 256. `Unmarshal` and `NewDecoder` use the
defaults; `NewDecoderWithOptions` gives a stream decoder its own options.

Every failure other than I/O and end-of-input is a `*cbor.Error` with a byte
offset, classified for `errors.Is`:

| Kind | Meaning |
|------|---------|
| `ErrMalformed` | not well-formed: reserved headers, a stray break, `f8 00`–`f8 1f`, trailing data |
| `ErrInvalid` | well-formed but invalid: duplicate map keys, invalid UTF-8 |
| `ErrUnsupported` | valid but outside this profile: indefinite lengths, composite or NaN keys, rejected tags, invalid struct tags |
| `ErrLimit` | over a configured limit |
| `ErrNotDeterministic` | rejected by `RejectNonMinimal` or `RequireDeterministic` |
| `ErrType` | valid, but not representable in the destination: type mismatch, overflow, unknown or missing required fields |

Duplicate keys are detected in the CBOR data model, so `1` and `0x1801`, or
`1.0` as binary16 and binary64, collide. The whole item is validated before
anything observable happens: application-provided `UnmarshalCBOR` methods run
only once the item, and the absence of trailing data, has been validated. A
destination is replaced on success and unchanged on error, apart from side
effects inside those methods.

Decoded byte slices, strings and `RawMessage` values never alias the input.
`RawMessage` copies the complete original encoding for delayed decoding.
Raw and custom-marshaled values are validated before encoding.

## Struct tags

| Tag | Meaning |
|-----|---------|
| `cbor:"name"` | text key `"name"`; the default is the field name |
| `cbor:"-7,keyasint"` | integer key −7 |
| ``_ struct{} `cbor:",keyasint"` `` | every field's name is an integer key; a field without one is an error |
| ``_ struct{} `cbor:",toarray"` `` | encode as an array of fields in declaration order, as COSE requires |
| `required` | decoding fails with `ErrType` when the key is absent |
| `omitempty` | omit false, 0, nil and empty values |
| `omitzero` | omit zero values, or values whose `IsZero() bool` reports true |
| `cbor:"-"` | ignore the field |

Decoding prefers exact keys, then unambiguous case-insensitive text matches.
`DecoderOptions.RejectUnknownFields` fails on keys that match no field and
turns off case-insensitive matching. Together with `required` and
`RequireDeterministic`, it accepts exactly one encoding of each message, which
replaces re-encoding a decoded message to check its shape.

Exported fields of embedded structs are promoted, including through embedded
pointers (allocated when decoding, skipped when nil), as in `encoding/json`. A
shallower field hides a deeper one with the same key; two fields with one key
at the same depth are an error rather than a silently dropped field. Struct
definition errors are `ErrUnsupported` on first use.

`Tag[T]` carries a tag with typed content, such as `Tag[Sign1]` for a
COSE_Sign1 message; `TaggedValue` keeps untyped content.

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
`AppendMarshal` appends to a caller's buffer; a struct encoded through it
allocates nothing.

## Diagnostic notation

`Diagnose` renders one item in [RFC 8949 §8](https://www.rfc-editor.org/rfc/rfc8949.html#section-8)
notation, bounded by `DecoderOptions`, for error messages, logs and tests. It
reproduces every RFC 8949 Appendix A example verbatim: `{1: -7, 4: h'3131'}`,
`1.0e+300`, `2(h'010000000000000000')`, `simple(16)`.

## Supported values and compatibility

- Integers of every size: `uint64`, `int64`, and `big.Int` or `*big.Int`,
  which use preferred serialization (an integer when the value fits in 64
  bits, otherwise tags 2 and 3 without leading zeros). Decoding into `any`
  yields `uint64`, `int64`, or `*big.Int` below `math.MinInt64`.
- Booleans, null, undefined and other simple values, UTF-8 text, byte strings,
  arrays, maps, and binary16/32/64 floats. Integer-to-float decoding succeeds
  only when exact; float-to-integer coercion is not supported.
- Null decodes to nil pointers, interfaces, maps, and slices, and is an error
  for other types. Undefined and unassigned simple values decode as
  `SimpleValue`.
- Tags are preserved as `TaggedValue` or `Tag[T]`. Bignum tags decode into
  `big.Int`; other date, URI and similar tags are not interpreted. `RejectTags`
  disables tags altogether.
- Generic maps use `map[any]any`. Composite and NaN map keys are unsupported.
  Byte-string keys can be kept with `RawMessage` or typed maps, but not as
  `map[any]any` keys; integer keys below `math.MinInt64` cannot be keys of any
  interface-keyed map, where a `*big.Int` would compare by pointer.
- `[]byte` and byte arrays such as `[16]byte` encode as byte strings. Byte
  strings decode into `[]byte` and into byte arrays of exactly matching length.
- Encoders cap items at 1 MiB, nesting at 64 levels, and arrays/maps at 10,000
  entries. Encoders validate and buffer an item before writing it; a failed
  writer can still leave partial output, and short writes return
  `io.ErrShortWrite`.

The module requires Go 1.26 and has no dependencies.

## Performance

Decoding validates and decodes in one pass with per-type decoders compiled
once and cached, and builds no index of the input; calls into `UnmarshalCBOR`
methods are queued and run once the whole item is valid, so a failure is
still invisible. Unknown fields, raw messages and custom-unmarshaled items are
checked by a validating scan. Encoding uses per-type encoders, struct keys
pre-sorted for each order, and pooled scratch buffers for map entries. The
only steady-state allocations are the decoded strings and byte slices, which
never alias the input, and the returned encoding.

Side by side with fxamacker/cbor v2.10.0 (strict duplicate-key and UTF-8
checks, core deterministic encoding), as benchstat medians of ten runs on a
shared linux/amd64 VM. "~" marks a difference that is not statistically
significant.

| Benchmark | picatz/cbor | fxamacker/cbor | Time |
|-----------|-------------|----------------|------|
| Decode RFC 8392 CWT claims into a struct | 590 ns, 5 allocs | 589 ns, 5 allocs | ~ |
| Decode an RFC 9052 EC2 COSE_Key | 472 ns, 4 allocs | 490 ns, 4 allocs | −4% |
| Decode a CTAP2 `getInfo` response into `any` | 2.93 µs, 52 allocs | 3.10 µs, 52 allocs | ~ |
| Encode CWT claims | 198 ns, 1 alloc | 196 ns, 1 alloc | ~ |
| Encode a COSE_Key | 205 ns, 1 alloc | 195 ns, 1 alloc | +5% |
| Encode WebAuthn-shaped text (five 15–60 byte strings) | 268 ns, 1 alloc | 221 ns, 1 alloc | +18% |
| Encode `omitzero` fields, two of them `time.Time` | 189 ns, 1 alloc | 179 ns, 1 alloc | ~ |
| Encode the `getInfo` response from `any` | 1.37 µs, 1 alloc | 1.50 µs, 1 alloc | −9% |
| Encode a `map[string]bool` | 547 ns, 1 alloc | 529 ns, 1 alloc | +3% |

Most of the remaining encoding gap is UTF-8 validation, which text encoding checks
eight bytes at a time: this package refuses to encode a Go string that is
not valid UTF-8, because CBOR text must be, while fxamacker/cbor encodes it
as written, producing items its own decoder rejects. There is deliberately
no option to skip the check.

Reproduce with `cd interop && go test -run '^$' -bench . -benchmem`, and
compare revisions with [benchstat](https://pkg.go.dev/golang.org/x/perf/cmd/benchstat).

## Validation

```sh
go test -race ./...
go vet ./...
go test -run '^$' -fuzz FuzzDecode -fuzztime 30s   # also FuzzRoundTrip, FuzzStream, FuzzShortestFloat, FuzzDiagnose, FuzzSchema, FuzzCOSE
env -i PATH="$PATH" HOME="$HOME" GOOS=js GOARCH=wasm go test -exec="$(go env GOROOT)/lib/wasm/go_js_wasm_exec" ./...
(cd interop && go test ./... && go test -run '^$' -fuzz FuzzDifferential -fuzztime 30s)
```

Conformance tests cover every RFC 8949 Appendix A example, its diagnostic
notation, and the deterministic-encoding round trip the RFC marks for each.
All 306 messages from the public-domain
[cose-wg/Examples](https://github.com/cose-wg/Examples) corpus are checked for
acceptance, COSE structure, byte-for-byte typed round trips, and deterministic
fixed points.
The separate `interop` module differentially tests decoding, validity
verdicts, re-encoding and diagnostic notation against fxamacker/cbor, and
compares core deterministic output byte for byte; it is a separate module so
this one keeps no dependencies. Fuzzers check round trips, deterministic fixed
points, stream splitting and truncation, shortest-float selection, strict
struct decoding, and that failed decodes leave destinations unchanged. CI runs
them on every push and for ten minutes each nightly, alongside `govulncheck`,
with actions pinned to commit SHAs.
