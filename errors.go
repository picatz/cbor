package cbor

import (
	"errors"
	"strconv"
)

// Error kinds. Every encoding and decoding failure other than I/O errors,
// io.EOF and io.ErrUnexpectedEOF is an *Error whose Kind is one of these, so
// callers can classify failures with errors.Is.
var (
	// ErrMalformed reports input that is not well-formed CBOR, such as a
	// reserved additional-information value, a two-byte simple value below
	// 32, or trailing bytes after a single item. See RFC 8949 Appendix F.
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#appendix-F
	ErrMalformed = errors.New("cbor: malformed")

	// ErrInvalid reports well-formed CBOR that is not valid, such as a map
	// with duplicate keys or a text string with invalid UTF-8.
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-5.3
	ErrInvalid = errors.New("cbor: invalid")

	// ErrUnsupported reports valid CBOR outside this package's profile, such as
	// indefinite lengths, composite or NaN map keys, negative integers below
	// math.MinInt64, tags when DecoderOptions.RejectTags is set, and Go types
	// without a CBOR mapping.
	ErrUnsupported = errors.New("cbor: unsupported")

	// ErrLimit reports an item that exceeds a configured resource limit. Limits
	// are checked before the corresponding allocation or recursion.
	ErrLimit = errors.New("cbor: limit exceeded")

	// ErrNotDeterministic reports input rejected by
	// DecoderOptions.RejectNonMinimal or DecoderOptions.RequireDeterministic,
	// or an embedded raw item that cannot be part of deterministic output.
	//
	// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.1
	ErrNotDeterministic = errors.New("cbor: not deterministic")

	// ErrType reports a valid item that cannot be stored in the destination
	// Go type without a type mismatch, loss of precision, or overflow.
	ErrType = errors.New("cbor: type mismatch")
)

// Error describes an encoding or decoding failure.
type Error struct {
	// Kind is one of the package's Err* values.
	Kind error

	// Offset is the byte offset, from the start of the item being decoded,
	// at which the problem was detected. It is -1 for encoding errors.
	Offset int64

	// Msg describes the problem without the "cbor: " prefix.
	Msg string
}

// Error implements the error interface.
func (err *Error) Error() string {
	if err.Offset < 0 {
		return "cbor: " + err.Msg
	}
	return "cbor: " + err.Msg + " at offset " + strconv.FormatInt(err.Offset, 10)
}

// Unwrap returns Kind so errors.Is can match the error kind.
func (err *Error) Unwrap() error { return err.Kind }

func newError(kind error, offset int, msg string) *Error {
	return &Error{Kind: kind, Offset: int64(offset), Msg: msg}
}

func encodeError(kind error, msg string) *Error {
	return &Error{Kind: kind, Offset: -1, Msg: msg}
}
