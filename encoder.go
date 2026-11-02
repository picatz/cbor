package cbor

import (
	"bytes"
	"errors"
	"io"
)

// SortMode selects the order of encoded map keys. Struct fields are encoded
// as maps and follow the same order.
type SortMode uint8

const (
	// SortLengthFirst orders keys by encoded length, then bytewise. It is the
	// default and matches the "canonical CBOR" ordering of RFC 7049
	// Section 3.9.
	//
	// https://www.rfc-editor.org/rfc/rfc7049.html#section-3.9
	SortLengthFirst SortMode = iota

	// SortBytewiseLexical orders keys bytewise by their encodings, as required
	// by RFC 8949 core deterministic encoding. Because keys use preferred
	// serialization, this is also the CTAP2 canonical CBOR key order (major
	// type, then encoded length, then bytewise).
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1
	// https://fidoalliance.org/specs/fido-v2.1-ps-20210615/fido-client-to-authenticator-protocol-v2.1-ps-errata-20220621.html#ctap2-canonical-cbor-encoding-form
	SortBytewiseLexical
)

// FloatMode selects how floating-point values are encoded.
type FloatMode uint8

const (
	// FloatBinary64 encodes every float32 and float64 value as binary64. It is
	// the default.
	FloatBinary64 FloatMode = iota

	// FloatShortest encodes each value in the shortest of binary16, binary32
	// and binary64 that preserves it exactly, as RFC 8949 preferred
	// serialization requires. Every NaN is encoded as 0xf97e00, so NaN
	// payloads are not preserved.
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.1
	FloatShortest
)

// EncoderOptions configures encoding, in the style of protobuf's
// MarshalOptions: a plain value whose zero value is the default used by
// Marshal and NewEncoder, and whose methods encode with it. Every mode writes
// definite lengths and the shortest integer, length and tag headers.
//
// Every mode also validates what it writes, so its output always decodes.
// In particular, a Go string that is not valid UTF-8 fails with ErrInvalid,
// because CBOR text must be UTF-8 (RFC 8949 Section 3.1); arbitrary bytes
// belong in a []byte, which encodes as a byte string.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.1
type EncoderOptions struct {
	// Sort selects the map key order. Struct fields follow it too.
	Sort SortMode

	// Floats selects floating-point widths.
	Floats FloatMode
}

// CoreDeterministicEncoding returns options that produce RFC 8949 core
// deterministic encoding: preferred serialization, shortest floating-point
// values and bytewise lexicographic map key order. Embedded RawMessage and
// Marshaler output must already be deterministic, or encoding fails with
// ErrNotDeterministic. DecoderOptions.RequireDeterministic accepts exactly
// this encoding.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1
func CoreDeterministicEncoding() EncoderOptions {
	return EncoderOptions{Sort: SortBytewiseLexical, Floats: FloatShortest}
}

func (options EncoderOptions) validate() error {
	if options.Sort > SortBytewiseLexical || options.Floats > FloatShortest {
		return errors.New("cbor: invalid encoder options")
	}
	return nil
}

func (options EncoderOptions) deterministic() bool {
	return options == CoreDeterministicEncoding()
}

// Encoder writes one item per Encode call.
type Encoder struct {
	writer  io.Writer
	options EncoderOptions
}

// NewEncoder constructs an encoder with default options. Each item is bounded
// to 1 MiB and 64 levels.
func NewEncoder(writer io.Writer) *Encoder { return &Encoder{writer: writer} }

// NewEncoderWithOptions constructs an encoder with the given options.
func NewEncoderWithOptions(writer io.Writer, options EncoderOptions) *Encoder {
	return &Encoder{writer: writer, options: options}
}

// Marshal encodes a value with default options into a caller-owned byte
// slice. Map keys use SortLengthFirst and floats use FloatBinary64; use
// CoreDeterministicEncoding for RFC 8949 deterministic output.
func Marshal(value any) ([]byte, error) { return EncoderOptions{}.Marshal(value) }

// AppendMarshal appends the encoding of value with default options to dst
// and returns the extended slice, so framing and repeated encoding can reuse
// one buffer. On error it returns dst unchanged, although bytes beyond
// len(dst) may have been written.
func AppendMarshal(dst []byte, value any) ([]byte, error) {
	return EncoderOptions{}.AppendMarshal(dst, value)
}

// Marshal encodes a value with these options into a caller-owned byte slice.
func (options EncoderOptions) Marshal(value any) ([]byte, error) {
	if err := options.validate(); err != nil {
		return nil, err
	}
	state := getEncodeState(options)
	defer putEncodeState(state)
	encoded, err := state.appendItem(state.buffer[:0], value)
	if err != nil {
		return nil, err
	}
	state.buffer = encoded[:0]
	return bytes.Clone(encoded), nil
}

// AppendMarshal appends the encoding of value with these options to dst.
// The 1 MiB item limit applies to the appended item alone.
func (options EncoderOptions) AppendMarshal(dst []byte, value any) ([]byte, error) {
	if err := options.validate(); err != nil {
		return dst, err
	}
	state := getEncodeState(options)
	defer putEncodeState(state)
	encoded, err := state.appendItem(dst, value)
	if err != nil {
		return dst, err
	}
	return encoded, nil
}

// Encode validates and buffers one complete item before writing it. A writer
// failure can still leave partial output and is returned to the caller.
func (encoder *Encoder) Encode(value any) error {
	if encoder.writer == nil {
		return errors.New("cbor: nil writer")
	}
	if err := encoder.options.validate(); err != nil {
		return err
	}
	state := getEncodeState(encoder.options)
	defer putEncodeState(state)
	encoded, err := state.appendItem(state.buffer[:0], value)
	if err != nil {
		return err
	}
	state.buffer = encoded[:0]
	written, err := encoder.writer.Write(encoded)
	if err == nil && written != len(encoded) {
		err = io.ErrShortWrite
	}
	return err
}
