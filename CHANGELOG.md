# Changelog

This project follows [Semantic Versioning](https://semver.org). Before v1,
minor versions may change behavior; such changes are listed under
**Changed**.

## Unreleased

### Added

- Struct-wide integer keys: a blank field ``_ struct{} `cbor:",keyasint"` ``
  makes every field name an integer key.
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
  buffer.
- Constants `DefaultMaxNestedLevels`, `DefaultMaxEncodedBytes` and
  `MaxNestedLevelsLimit` name the documented limits.
- Fuzzers `FuzzDiagnose` and `FuzzSchema`; a nightly fuzzing workflow;
  `govulncheck`; actions pinned to commit SHAs; Dependabot.
- Side-by-side benchmarks against fxamacker/cbor in the `interop` module.

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
- Decoding and encoding are rebuilt on a pooled parse arena and per-type
  compiled codecs: about 40% less time, 69% fewer bytes and 46% fewer
  allocations across the package benchmarks.

### Deprecated

- `DefaultDecoderOptions`: a mutable global races with concurrent decoding.
  Use a `DecoderOptions` value; its zero value selects the same defaults.
- `Decoder.SetMax`, `SetMaxArrayElements`, `SetMaxMapPairs`,
  `SetMaxStringBytes` and `SetMaxBytes`: use `NewDecoderWithOptions`.

### Notes toward v1

- Options stay plain values with methods (`DecoderOptions.Unmarshal`,
  `EncoderOptions.Marshal`), like protobuf's `UnmarshalOptions` and
  `MarshalOptions`, rather than compiled `DecMode`/`EncMode` objects: the
  per-type work is already cached, so a mode object would add API without
  saving time.
- v1 should remove the deprecated global and setters, leaving `Unmarshal`
  and `NewDecoder` with the zero-value defaults.
