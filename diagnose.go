package cbor

import (
	"encoding/hex"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Diagnose returns the diagnostic notation of exactly one item, as defined
// by RFC 8949 Section 8, using the default decoder limits. It is meant for
// error messages, logs and tests; for example, the item a2 01 26 04 42 31 31
// is {1: -7, 4: h'3131'}.
//
// Byte strings are written in base16, floating-point values as in RFC 8949
// Appendix A (1.0, 1.0e+300, Infinity, NaN), and tags and simple values as
// 1(...) and simple(16).
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-8
func Diagnose(data []byte) (string, error) {
	return DecoderOptions{}.Diagnose(data)
}

// Diagnose returns the diagnostic notation of exactly one item, bounded by
// these options. Input the options reject, such as non-deterministic input
// with RequireDeterministic, fails with the same error Unmarshal returns.
func (options DecoderOptions) Diagnose(data []byte) (string, error) {
	options, err := options.normalized()
	if err != nil {
		return "", err
	}
	state := getState(&options)
	defer putState(state)
	parser := parser{input: data, options: options, items: state.items}
	root, err := parser.parse(0)
	state.items = parser.items
	if err != nil {
		return "", err
	}
	if len(parser.data) != len(data) {
		return "", newError(ErrMalformed, len(parser.data), "trailing data")
	}
	state.data = parser.data
	var builder strings.Builder
	state.diagnose(&builder, root)
	return builder.String(), nil
}

// diagnose writes the notation of the item at index and returns the index
// of its next sibling.
func (state *decodeState) diagnose(builder *strings.Builder, index int) int {
	value := &state.items[index]
	var scratch [32]byte
	switch value.major {
	case 0:
		builder.Write(strconv.AppendUint(scratch[:0], value.argument, 10))
	case 1:
		if value.argument == math.MaxUint64 {
			builder.WriteString("-18446744073709551616")
		} else {
			builder.WriteByte('-')
			builder.Write(strconv.AppendUint(scratch[:0], value.argument+1, 10))
		}
	case 2:
		builder.WriteString("h'")
		builder.WriteString(hex.EncodeToString(state.payload(value)))
		builder.WriteByte('\'')
	case 3:
		quote(builder, state.payload(value))
	case 4, 5:
		open, close := "[", "]"
		if value.major == 5 {
			open, close = "{", "}"
		}
		builder.WriteString(open)
		child := index + 1
		for position := range value.argument {
			if position > 0 {
				builder.WriteString(", ")
			}
			child = state.diagnose(builder, child)
			if value.major == 5 {
				builder.WriteString(": ")
				child = state.diagnose(builder, child)
			}
		}
		builder.WriteString(close)
	case 6:
		builder.Write(strconv.AppendUint(scratch[:0], value.argument, 10))
		builder.WriteByte('(')
		state.diagnose(builder, index+1)
		builder.WriteByte(')')
	default:
		switch {
		case value.isFloat():
			builder.WriteString(formatFloat(value.float()))
		case value.minor == 20:
			builder.WriteString("false")
		case value.minor == 21:
			builder.WriteString("true")
		case value.minor == 22:
			builder.WriteString("null")
		case value.minor == 23:
			builder.WriteString("undefined")
		default:
			builder.WriteString("simple(")
			builder.Write(strconv.AppendUint(scratch[:0], value.argument, 10))
			builder.WriteByte(')')
		}
	}
	return value.next
}

// quote writes text as a JSON-style string literal, which RFC 8949 Section 8
// adopts for text strings.
func quote(builder *strings.Builder, text []byte) {
	const digits = "0123456789abcdef"
	builder.WriteByte('"')
	for len(text) > 0 {
		character, size := utf8.DecodeRune(text)
		switch {
		case character == '"' || character == '\\':
			builder.WriteByte('\\')
			builder.WriteByte(byte(character))
		case character == '\n':
			builder.WriteString(`\n`)
		case character == '\r':
			builder.WriteString(`\r`)
		case character == '\t':
			builder.WriteString(`\t`)
		case character < 0x20 || character == 0x7f:
			builder.WriteString(`\u00`)
			builder.WriteByte(digits[character>>4])
			builder.WriteByte(digits[character&15])
		default:
			builder.Write(text[:size])
		}
		text = text[size:]
	}
	builder.WriteByte('"')
}

// formatFloat writes a number the way RFC 8949 Appendix A does: the
// shortest decimal that round-trips, in exponent form when the exponent is
// below -6 or above 20 (as in ECMAScript), and always with a fraction.
func formatFloat(number float64) string {
	switch {
	case math.IsNaN(number):
		return "NaN"
	case math.IsInf(number, 1):
		return "Infinity"
	case math.IsInf(number, -1):
		return "-Infinity"
	}
	scientific := strconv.FormatFloat(number, 'e', -1, 64)
	mantissa, exponentText, _ := strings.Cut(scientific, "e")
	exponent, _ := strconv.Atoi(exponentText)
	if exponent >= -6 && exponent < 21 {
		fixed := strconv.FormatFloat(number, 'f', -1, 64)
		if !strings.Contains(fixed, ".") {
			fixed += ".0"
		}
		return fixed
	}
	if !strings.Contains(mantissa, ".") {
		mantissa += ".0"
	}
	sign := "+"
	if exponent < 0 {
		sign, exponent = "-", -exponent
	}
	return mantissa + "e" + sign + strconv.Itoa(exponent)
}
