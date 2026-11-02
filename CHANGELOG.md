# Changelog

This project follows [Semantic Versioning](https://semver.org). Before v1,
minor versions may change behavior; such changes are listed under
**Changed**.

## Unreleased

Changes since `f5df8ee`, the last revision before
[#12](https://github.com/picatz/cbor/pull/12).

### Added

- Struct-wide integer keys: a blank field ``_ struct{} `cbor:",keyasint"` ``
  makes every field name an integer key, as COSE and CWT use.
- The `required` field option fails decoding with `ErrType` when the key is
  absent. With `RejectUnknownFields` and `RequireDeterministic` it accepts
  exactly one encoding of a message.
- Embedded struct fields are promoted, including through embedded pointers.
- `big.Int` and `*big.Int` encode and decode with RFC 8949 Section 3.4.3
  preferred serialization. `RequireDeterministic` rejects bignums with leading
  zeros or values that fit in an integer.
- `Diagnose` and `DecoderOptions.Diagnose` render RFC 8949 Section 8
  diagnostic notation.
- `Decoder.All` iterates an RFC 8742 CBOR sequence.
- `AppendMarshal` and `EncoderOptions.AppendMarshal` append to a caller's
  buffer; a struct encodes without allocating.
- Constants `DefaultMaxNestedLevels`, `DefaultMaxEncodedBytes` and
  `MaxNestedLevelsLimit` name the documented limits.

### Changed

- The module requires Go 1.26.
- Exported embedded structs are flattened into their parent, as in
  `encoding/json`, instead of encoding as a nested map under the type name.
  Exported fields of unexported embedded structs are now promoted too. Give an
  embedded field a tag name to keep the old shape.
- Two fields with the same key at the same depth are an error, including a
  tagged and an untagged field. Struct definition errors are `*Error` values
  with kind `ErrUnsupported`.
- Negative integers below `math.MinInt64` are accepted instead of failing with
  `ErrUnsupported`. They decode into `big.Int`, `*big.Int` or `RawMessage`, and
  into `any` as `*big.Int`; into `int64` they fail with `ErrType`. As keys of
  maps with interface-typed keys, such as `map[any]any`, they fail with
  `ErrType`, because a `*big.Int` key would compare by pointer.
- Type error messages name the item and destination, such as `cannot decode
  text string into int` or `negative integer overflows uint8`.
- `MaxEncodedBytes` above `math.MaxInt32` is an invalid option.
- Decoding and encoding are rebuilt on per-type compiled codecs. Decoding
  validates and decodes in one pass and builds no index of the input;
  `UnmarshalCBOR` calls are queued and run once the whole item is valid, so
  a failure still leaves the destination unchanged. Struct decoding matches
  fxamacker/cbor v2.10.0 with strict checks enabled.
- Duplicate map keys are found through per-map key fingerprints on the
  stack, so a full comparison runs only for keys that may be equal; maps
  decoded into Go maps detect them through the map itself.
- Encoding a struct with omitted fields writes it in one pass, and
  encoding a reflected map reuses its iteration values, so every encoding
  allocates only its result.
- Text strings are checked for valid UTF-8 eight bytes at a time before
  falling back to `unicode/utf8`, so encoding text-heavy structs is about
  12% faster; invalid UTF-8 still fails with `ErrInvalid`.
- Struct encoders iterate precomputed field plans, locating each field
  with one `reflect.Value.Field` call.
- `omitzero` resolves a type's `IsZero` method once per field instead of on
  every encode, and calls it through a pointer when the field is
  addressable, so it no longer boxes values such as `time.Time`: encoding
  such structs is about 4 to 5 times faster. As in `encoding/json`, an
  `IsZero` method on the pointer type is honored for addressable fields.
- The decoder and encoder remember the type they last handled on their
  pooled state rather than in a shared variable, which is about 10% faster
  when goroutines handle different types concurrently.
- The densest adversarial input, one-byte items, allocates about one byte
  per input byte instead of about 182.

### Removed

- `DefaultDecoderOptions`, a mutable global that raced with concurrent
  decoding. `Unmarshal` and `NewDecoder` use the defaults; replace
  `cbor.DefaultDecoderOptions.Unmarshal(data, v)` with
  `cbor.DecoderOptions{...}.Unmarshal(data, v)` for other limits.
- `Decoder.SetMax`, `SetMaxArrayElements`, `SetMaxMapPairs`,
  `SetMaxStringBytes` and `SetMaxBytes`. Replace
  `d := cbor.NewDecoder(r); d.SetMaxBytes(n)` with
  `d := cbor.NewDecoderWithOptions(r, cbor.DecoderOptions{MaxBytes: n})`.

`go fix` cannot make these replacements: its inliner needs a function or
constant whose body can stand in for the call, and neither a mutable
variable nor a setter of unexported fields has one.

### Testing and CI

- Every message in the public-domain cose-wg/Examples corpus is checked, and
  the `interop` module compares it, and diagnostic notation, against
  fxamacker/cbor.
- New fuzzers `FuzzDiagnose`, `FuzzSchema` and `FuzzCOSE`, and every fuzzer
  runs for ten minutes nightly.
- Actions are pinned to commit SHAs and updated by Dependabot, and
  `govulncheck` runs on both modules.
- Side-by-side benchmarks against fxamacker/cbor (now v2.10.0) live in the
  `interop` module.

### Design notes toward v1

- Options stay plain values with methods (`DecoderOptions.Unmarshal`,
  `EncoderOptions.Marshal`), like protobuf's `UnmarshalOptions` and
  `MarshalOptions`, rather than compiled `DecMode`/`EncMode` objects: the
  per-type work is already cached, so a mode object would add API without
  saving time.
- SIMD and assembly are deferred: profiles put decoding cost in parsing,
  reflection and allocation, and Go's `simd/archsimd` is still experimental
  ([#7](https://github.com/picatz/cbor/issues/7)).
