package cbor

import "math"

// canonicalNaN is the binary16 quiet NaN that deterministic encoding uses for
// every NaN value, as recommended by RFC 8949 Section 4.2.2.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.2.2
const canonicalNaN = 0x7e00

// halfFloat converts IEEE 754 binary16 bits to a float64.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#appendix-D
func halfFloat(bits uint16) float64 {
	exponent, fraction := (bits>>10)&31, bits&1023
	var value float64
	switch exponent {
	case 0:
		value = math.Ldexp(float64(fraction), -24)
	case 31:
		if fraction == 0 {
			value = math.Inf(1)
		} else {
			value = math.NaN()
		}
	default:
		value = math.Ldexp(float64(fraction+1024), int(exponent)-25)
	}
	if bits&0x8000 != 0 {
		value = -value
	}
	return value
}

// toHalf returns the binary16 encoding of value when it is exactly
// representable. NaN values are never reported as representable.
func toHalf(value float64) (uint16, bool) {
	if value != value {
		return 0, false
	}
	var sign uint16
	if math.Signbit(value) {
		sign = 0x8000
	}
	magnitude := math.Abs(value)
	var candidate uint16
	switch {
	case magnitude == 0:
		return sign, true
	case math.IsInf(magnitude, 0):
		return sign | 0x7c00, true
	case magnitude > 65504:
		return 0, false
	case magnitude < 0x1p-14:
		scaled := magnitude * 0x1p24
		if scaled != math.Trunc(scaled) {
			return 0, false
		}
		candidate = sign | uint16(scaled)
	default:
		single := float32(magnitude)
		if float64(single) != magnitude {
			return 0, false
		}
		bits := math.Float32bits(single)
		exponent := int((bits>>23)&0xff) - 127
		candidate = sign | uint16(exponent+15)<<10 | uint16(bits>>13)&0x3ff
	}
	if math.Float64bits(halfFloat(candidate)) != math.Float64bits(value) {
		return 0, false
	}
	return candidate, true
}

// shortestFloat returns the additional-information value (25, 26 or 27) and
// argument bits of the shortest encoding that preserves value exactly, as
// required by RFC 8949 preferred serialization. Every NaN becomes canonicalNaN.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-4.1
func shortestFloat(value float64) (byte, uint64) {
	if value != value {
		return 25, canonicalNaN
	}
	if half, ok := toHalf(value); ok {
		return 25, uint64(half)
	}
	if single := float32(value); float64(single) == value {
		return 26, uint64(math.Float32bits(single))
	}
	return 27, math.Float64bits(value)
}
