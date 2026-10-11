package cbor

import (
	"bytes"
	"errors"
)

// RawMessage retains one encoded CBOR item, including its original header.
type RawMessage []byte

// MarshalCBOR returns a validated encoding. A nil RawMessage encodes as null.
func (message RawMessage) MarshalCBOR() ([]byte, error) {
	if message == nil {
		return []byte{0xf6}, nil
	}
	if err := validateRaw(message); err != nil {
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

func validateRaw(data []byte) error {
	return validateRawAtDepth(data, 0)
}

func validateRawAtDepth(data []byte, depth int) error {
	options, err := DefaultDecoderOptions.normalized()
	if err != nil {
		return err
	}
	if options.MaxNestedLevels > 64 {
		options.MaxNestedLevels = 64
	}
	reader := bytes.NewReader(data)
	parser := parser{reader: reader, options: options}
	if _, err := parser.item(depth); err != nil {
		return err
	}
	if reader.Len() != 0 {
		return errors.New("cbor: trailing raw data")
	}
	return nil
}
