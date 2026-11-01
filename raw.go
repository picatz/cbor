package cbor

import (
	"errors"
	"io"
)

// RawMessage retains one encoded CBOR item, including its original header.
// It supports delayed decoding and embedding pre-encoded items.
type RawMessage []byte

// MarshalCBOR returns a validated encoding. A nil RawMessage encodes as null.
func (message RawMessage) MarshalCBOR() ([]byte, error) {
	if message == nil {
		return []byte{0xf6}, nil
	}
	if err := validateRaw(message, 0, false); err != nil {
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

// rawOptions validates embedded raw items and encoded map keys with limits
// matching the encoder's own.
var rawOptions = DecoderOptions{
	MaxArrayElements: DefaultMaxValue, MaxMapPairs: DefaultMaxValue,
	MaxStringBytes: maxEncodedItem, MaxBytes: maxEncodedItem,
	MaxNestedLevels: maxEncodedDepth, MaxEncodedBytes: maxEncodedItem,
}

// rawDeterministicOptions additionally requires core deterministic encoding.
var rawDeterministicOptions = func() DecoderOptions {
	options := rawOptions
	options.RequireDeterministic, options.RejectNonMinimal = true, true
	return options
}()

// validateRaw checks that data is exactly one item that fits in the remaining
// nesting budget below depth and, when deterministic is set, that it is in
// core deterministic encoding.
func validateRaw(data []byte, depth int, deterministic bool) error {
	options := rawOptions
	if deterministic {
		options = rawDeterministicOptions
	}
	state := getState(&options)
	defer putState(state)
	parser := parser{input: data, options: options, items: state.items}
	_, err := parser.parse(depth)
	state.items = parser.items
	if err != nil {
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
