package cbor

import (
	"errors"
	"io"
)

// RawMessage retains one encoded CBOR item, including its original header.
type RawMessage []byte

// MarshalCBOR returns a validated encoding. A nil RawMessage encodes as null.
func (message RawMessage) MarshalCBOR() ([]byte, error) {
	if message == nil {
		return []byte{0xf6}, nil
	}
	if err := validateRawAtDepth(message, 0, false); err != nil {
		return nil, err
	}
	return message, nil
}

// UnmarshalCBOR copies data so the result does not alias the decoder's buffer.
func (message *RawMessage) UnmarshalCBOR(data []byte) error {
	if message == nil {
		return errors.New("cbor: nil RawMessage receiver")
	}
	*message = append((*message)[:0], data...)
	return nil
}

// rawOptions validates embedded raw items and encoded map keys. It is fixed so
// that encoding never depends on the mutable DefaultDecoderOptions.
var rawOptions = DecoderOptions{
	MaxArrayElements: DefaultMaxValue, MaxMapPairs: DefaultMaxValue,
	MaxStringBytes: maxEncodedItem, MaxBytes: maxEncodedItem,
	MaxNestedLevels: maxEncodedDepth, MaxEncodedBytes: maxEncodedItem,
}

// validateRawAtDepth checks that data is exactly one item that fits in the
// remaining nesting budget and, when deterministic is set, that it is in core
// deterministic encoding.
func validateRawAtDepth(data []byte, depth int, deterministic bool) error {
	options := rawOptions
	options.RequireDeterministic = deterministic
	options, err := options.normalized()
	if err != nil {
		return err
	}
	parser := parser{input: data, options: options}
	if _, err := parser.item(depth); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return err
	}
	if len(parser.data) != len(data) {
		return newError(ErrMalformed, len(parser.data), "trailing raw data")
	}
	return nil
}
