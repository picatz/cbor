package cbor

import (
	"math/big"
	"reflect"
)

// Bignums (RFC 8949 Section 3.4.3) map to big.Int and *big.Int. Encoding uses
// preferred serialization: an integer of major type 0 or 1 when the value
// fits in 64 bits, and otherwise tag 2 or 3 around the big-endian magnitude
// without leading zero bytes. Decoding accepts both forms.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.4.3

// negativeBig returns -1 - argument for a negative integer below
// math.MinInt64.
func negativeBig(argument uint64) *big.Int {
	value := new(big.Int).SetUint64(argument)
	return value.Not(value) // -1 - argument
}

func decodeBigInt(state *decodeState, destination reflect.Value, value item, depth int) error {
	result := destination.Addr().Interface().(*big.Int)
	switch value.major {
	case 0:
		result.SetUint64(value.argument)
		return nil
	case 1:
		result.SetUint64(value.argument)
		result.Not(result)
		return nil
	case 6:
		if value.argument != TagUnsignedBignum && value.argument != TagNegativeBignum {
			break
		}
		var content item
		if err := state.item(&content, depth+1); err != nil {
			return err
		}
		if content.major != 2 {
			break
		}
		magnitude := state.payload(&content)
		if state.deterministic && (len(magnitude) <= 8 || magnitude[0] == 0) {
			return newError(ErrNotDeterministic, value.start, "bignum not in preferred serialization")
		}
		result.SetBytes(magnitude)
		if value.argument == TagNegativeBignum {
			result.Not(result)
		}
		return nil
	}
	return state.typeError(&value, destination.Type())
}

// appendBigInt encodes a bignum in preferred serialization.
func appendBigInt(output []byte, value *big.Int) []byte {
	if value.Sign() >= 0 {
		if value.IsUint64() {
			return appendHeader(output, 0, value.Uint64())
		}
		return appendBignum(output, TagUnsignedBignum, value)
	}
	// The argument of a negative integer is -1 - value, which is |value| - 1.
	magnitude := new(big.Int).Not(value)
	if magnitude.IsUint64() {
		return appendHeader(output, 1, magnitude.Uint64())
	}
	return appendBignum(output, TagNegativeBignum, magnitude)
}

func appendBignum(output []byte, tag uint64, magnitude *big.Int) []byte {
	length := (magnitude.BitLen() + 7) / 8
	output = appendHeader(appendHeader(output, 6, tag), 2, uint64(length))
	start := len(output)
	output = append(output, make([]byte, length)...)
	magnitude.FillBytes(output[start:])
	return output
}
