package cbor

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"net/mail"
	"net/url"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// MajorType is the major type of a CBOR item.
//
// https://tools.ietf.org/html/rfc7049#section-2.1
type MajorType int

// Major types defined in RFC 7049.
const (
	// MajorTypeUnsignedInt is the major type for unsigned integers.
	MajorTypeUnsignedInt MajorType = 0 // uint

	// MajorTypeNegativeInt is the major type for negative integers.
	MajorTypeNegativeInt MajorType = 1 // int

	// MajorTypeByteString is the major type for byte strings.
	MajorTypeByteString MajorType = 2 // []byte

	// MajorTypeTextString is the major type for text strings.
	MajorTypeTextString MajorType = 3 // string

	// MajorTypeArray is the major type for arrays.
	MajorTypeArray MajorType = 4 // []any

	// MajorTypeMap is the major type for maps.
	MajorTypeMap MajorType = 5 // map[any]any

	// MajorTypeTag is the major type for tags.
	MajorTypeTag MajorType = 6 // tag

	// MajorTypeSimple is the major type for simple values.
	MajorTypeSimple MajorType = 7 // simple (bool, nil, etc.)
)

// SimpleValue is a simple value.
//
// https://tools.ietf.org/html/rfc7049#section-2.3
type SimpleValue int

// Simple values defined in RFC 7049.
const (
	// SimpleValueFalse is the simple value for false.
	SimpleValueFalse SimpleValue = 20

	// SimpleValueTrue is the simple value for true.
	SimpleValueTrue SimpleValue = 21

	// SimpleValueNull is the simple value for null.
	SimpleValueNull SimpleValue = 22

	// SimpleValueUndefined is the simple value for undefined.
	SimpleValueUndefined SimpleValue = 23

	// SimpleValueSimpleValue is the simple value for simple value.
	SimpleValueSimpleValue SimpleValue = 24

	// SimpleValueFloat16 is the simple value for a 16-bit float.
	SimpleValueFloat16 SimpleValue = 25

	// SimpleValueFloat32 is the simple value for a 32-bit float.
	SimpleValueFloat32 SimpleValue = 26

	// SimpleValueFloat64 is the simple value for a 64-bit float.
	SimpleValueFloat64 SimpleValue = 27

	// SimpleValueBreak is the simple value for break.
	SimpleValueBreak SimpleValue = 31
)

// Tag is a CBOR tag.
//
// https://tools.ietf.org/html/rfc7049#section-2.4
type Tag int

// Tags defined in RFC 7049.
const (
	// TagDateTimeString is the tag for a date/time string.
	TagDateTimeString Tag = 0

	// TagUnixTime is the tag for a Unix time.
	TagUnixTime Tag = 1

	// TagPositiveBignum is the tag for a positive bignum.
	TagPositiveBignum Tag = 2

	// TagNegativeBignum is the tag for a negative bignum.
	TagNegativeBignum Tag = 3

	// TagDecimalFraction is the tag for a decimal fraction.
	TagDecimalFraction Tag = 4

	// TagBigfloat is the tag for a bigfloat.
	TagBigfloat Tag = 5

	// TagBase64URL is the tag for a base64url-encoded string.
	TagBase64URL Tag = 21

	// TagBase64 is the tag for a base64-encoded string.
	TagBase64 Tag = 22

	// TagBase16 is the tag for a base16-encoded string.
	TagBase16 Tag = 23

	// TagCBOR is the tag for a CBOR-encoded value.
	TagCBOR Tag = 24

	// TagURI is the tag for a URI.
	TagURI Tag = 32

	// TagBase64URLNoPadding is the tag for a base64url-encoded string
	// without padding.
	TagBase64URLNoPadding Tag = 33

	// TagBase64NoPadding is the tag for a base64-encoded string without
	// padding.
	TagBase64NoPadding Tag = 34

	// TagRegularExpression is the tag for a regular expression.
	TagRegularExpression Tag = 35

	// TagMIMEMessage is the tag for a MIME message.
	TagMIMEMessage Tag = 36

	// TagCBORSequence is the tag for a CBOR sequence.
	TagCBORSequence Tag = 258

	// TagCBORMap is the tag for a CBOR map.
	TagCBORMap Tag = 259

	// TagCBORSet is the tag for a CBOR set.
	TagCBORSet Tag = 260

	// TagCBORDateTimeString is the tag for a CBOR date/time string.
	TagCBORDateTimeString Tag = 261

	// TagCBORUnixTime is the tag for a CBOR Unix time.
	TagCBORUnixTime Tag = 262

	// TagCBORPositiveBignum is the tag for a CBOR positive bignum.
	TagCBORPositiveBignum Tag = 263

	// TagCBORNegativeBignum is the tag for a CBOR negative bignum.
	TagCBORNegativeBignum Tag = 264

	// TagCBORDecimalFraction is the tag for a CBOR decimal fraction.
	TagCBORDecimalFraction Tag = 265

	// TagCBORBigfloat is the tag for a CBOR bigfloat.
	TagCBORBigfloat Tag = 266

	// TagCBORBase64URL is the tag for a CBOR base64url-encoded string.
	TagCBORBase64URL Tag = 267

	// TagCBORBase64 is the tag for a CBOR base64-encoded string.
	TagCBORBase64 Tag = 268

	// TagCBORBase16 is the tag for a CBOR base16-encoded string.
	TagCBORBase16 Tag = 269

	// TagCBORURI is the tag for a CBOR URI.
	TagCBORURI Tag = 270

	// TagCBORBase64URLNoPadding is the tag for a CBOR base64url-encoded
	// string without padding.
	TagCBORBase64URLNoPadding Tag = 271

	// TagCBORBase64NoPadding is the tag for a CBOR base64-encoded string
	// without padding.
	TagCBORBase64NoPadding Tag = 272

	// TagCBORRegularExpression is the tag for a CBOR regular expression.
	TagCBORRegularExpression Tag = 273

	// TagCBORMIMEMessage is the tag for a CBOR MIME message.
	TagCBORMIMEMessage Tag = 274
)

// Unmarshaler is the interface implemented by types that can unmarshal a CBOR
// description of themselves.
//
// The input can be assumed to be a valid encoding of a CBOR value. UnmarshalCBOR
// must copy the CBOR data if it wishes to retain the data after returning.
type Unmarshaler interface {
	UnmarshalCBOR([]byte) error
}

// Marshaler is the interface implemented by types that can marshal themselves
// into a CBOR description.
//
// MarshalCBOR must copy the CBOR data if it wishes to retain the data after
// returning.
type Marshaler interface {
	MarshalCBOR() ([]byte, error)
}

// Unmarshall unmarsalls the CBOR-encoded data and stores the result in the value
// pointed to by v.
//
// If v is nil or not a pointer, Unmarshal returns an InvalidUnmarshalError.
//
// If v is a pointer to a nil pointer, Unmarshal allocates a new value for it to
// point to.
//
// If v implements the Unmarshaler interface, Unmarshal calls its UnmarshalCBOR
// method with the CBOR data and returns its error, if any.
//
// Otherwise, if the CBOR data is a CBOR array, Unmarshal decodes the CBOR array
// into the slice pointed to by v. If v is not a pointer to a slice, Unmarshal
// returns an InvalidUnmarshalError.
//
// Otherwise, if the CBOR data is a CBOR map, Unmarshal decodes the CBOR map into
// the map pointed to by v. If v is not a pointer to a map, Unmarshal returns an
// InvalidUnmarshalError.
//
// Otherwise, Unmarshal decodes the CBOR data into the value pointed to by v. If
// v is not a pointer, Unmarshal returns an InvalidUnmarshalError.
func Unmarshal(data []byte, v interface{}) error {
	return NewDecoder(bytes.NewReader(data)).Decode(v)
}

// A Decoder reads and decodes CBOR values from an input stream.
//
// It is not safe to be called from multiple goroutines.
type Decoder struct {
	// contains filtered or unexported fields
	r io.Reader

	maxArrayElements int
	maxMapPairs      int
	maxStringBytes   int
	maxBytes         int
}

// DefaultMaxValue is the default maximum value for the decoder
// used for all limits. This is a generous value that should be
// sufficient for most use cases. If you need to decode larger
// values, you can increase the limit using the appropriate.
//
// If you do not need to decode large values, you can decrease
// the limit to reduce the memory usage of the decoder. This is
// also useful for mitigating DoS attacks.
const DefaultMaxValue = 1000000

// NewDecoder returns a new decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{
		r:                r,
		maxArrayElements: DefaultMaxValue,
		maxMapPairs:      DefaultMaxValue,
		maxStringBytes:   DefaultMaxValue,
		maxBytes:         DefaultMaxValue,
	}
}

// SetMax sets all the maximum values to n.
func (dec *Decoder) SetMax(n int) {
	dec.maxArrayElements = n
	dec.maxMapPairs = n
	dec.maxStringBytes = n
	dec.maxBytes = n
}

// SetMaxArrayElements sets the maximum number of elements in an array.
//
// If the number of elements in an array exceeds this limit, an error is
// returned.
//
// The default limit is 1,000,000.
func (dec *Decoder) SetMaxArrayElements(n int) {
	dec.maxArrayElements = n
}

// SetMaxMapPairs sets the maximum number of pairs in a map.
//
// If the number of pairs in a map exceeds this limit, an error is returned.
//
// The default limit is 1,000,000.
func (dec *Decoder) SetMaxMapPairs(n int) {
	dec.maxMapPairs = n
}

// SetMaxStringBytes sets the maximum number of bytes in a string.
//
// If the number of bytes in a string exceeds this limit, an error is
// returned.
//
// The default limit is 1,000,000.
func (dec *Decoder) SetMaxStringBytes(n int) {
	dec.maxStringBytes = n
}

// SetMaxBytes sets the maximum number of bytes in a byte string.
//
// If the number of bytes in a byte string exceeds this limit, an error is
// returned.
//
// The default limit is 1,000,000.
func (dec *Decoder) SetMaxBytes(n int) {
	dec.maxBytes = n
}

// Decode reads the next CBOR-encoded value from its input and stores
// it in the value pointed to by v.
//
// See the documentation for Unmarshal for details about the conversion of
// a CBOR value into a Go value.
func (dec *Decoder) Decode(v interface{}) error {
	// Check that v is a pointer and not nil.
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return errors.New("cbor: Decode(non-pointer " + rv.Type().String() + ")")
	}
	if rv.IsNil() {
		return errors.New("cbor: Decode(nil " + rv.Type().String() + ")")
	}
	// Decode the CBOR value into the value pointed to by v.
	err := dec.decodeValue(rv.Elem())
	if err != nil {
		return fmt.Errorf("cbor: Decode(%v): %v", rv.Type(), err)
	}
	return nil
}

// readByte reads a single byte from the input stream.
//
// This is the basic building block for all other CBOR decoding.
func (dec *Decoder) readByte() (byte, error) {
	var b [1]byte
	_, err := io.ReadFull(dec.r, b[:])
	if err != nil {
		return 0, err
	}
	return b[0], nil
}

// readHeader reads the header byte and returns the major type and additional
// information. This is called before obtaining the value of a CBOR item.
func (dec *Decoder) readHeader() (majorType, additionalInfo byte, err error) {
	b, err := dec.readByte()
	if err != nil {
		return 0, 0, err
	}
	return b >> 5, b & 0x1f, nil
}

// decodeValue decodes a CBOR value into the given reflect.Value.
func (dec *Decoder) decodeValue(rv reflect.Value) error {
	// Read the header, which contains the major type and additional
	// information about the value.
	mt, ai, err := dec.readHeader()
	if err != nil {
		return err
	}

	// Decode the value based on the major type.
	switch MajorType(mt) {
	case MajorTypeUnsignedInt:
		return dec.decodeUint(rv, ai)
	case MajorTypeNegativeInt:
		return dec.decodeInt(rv, ai)
	case MajorTypeByteString:
		return dec.decodeBytes(rv, ai)
	case MajorTypeTextString:
		return dec.decodeString(rv, ai)
	case MajorTypeArray:
		return dec.decodeArray(rv, ai)
	case MajorTypeMap:
		return dec.decodeMap(rv, ai)
	case MajorTypeTag:
		return dec.decodeTag(rv, ai)
	case MajorTypeSimple:
		return dec.decodeSimpleValue(rv, ai)
	default:
		return errors.New("cbor: invalid major type")
	}
}

// decodeSimpleValue decodes a CBOR simple value into the given reflect.Value.
func (dec *Decoder) decodeSimpleValue(rv reflect.Value, ai byte) error {
	// Decode the simple value based on the additional information.
	switch SimpleValue(ai) {
	case SimpleValueFalse:
		// If the reflect.Value is a pointer, when we can possibly
		// convert it to a bool.
		if rv.Kind() == reflect.Ptr && rv.Type().Elem().Kind() == reflect.Bool {
			rv.Set(reflect.New(rv.Type().Elem()))
			rv = rv.Elem()
		}
		rv.SetBool(false)
	case SimpleValueTrue:
		// If the reflect.Value is a pointer, when we can possibly
		// convert it to a bool.
		if rv.Kind() == reflect.Ptr && rv.Type().Elem().Kind() == reflect.Bool {
			rv.Set(reflect.New(rv.Type().Elem()))
			rv = rv.Elem()
		}
		rv.SetBool(true)
	case SimpleValueNull:
		rv.Set(reflect.Zero(rv.Type()))
	case SimpleValueUndefined:
	// Do nothing.
	case SimpleValueFloat16:
		return errors.New("cbor: float16 not supported") // TODO: Implement float16?
	case SimpleValueFloat32:
		f, err := dec.readFloat32()
		if err != nil {
			return err
		}

		switch rv.Kind() {
		case reflect.Float32:
			rv.SetFloat(float64(f))
		case reflect.Float64:
			rv.SetFloat(float64(f))
		case reflect.Pointer:
			// If the reflect.Value is a pointer, when we can possibly
			// convert it to a float32 or float64.
			switch rv.Type().Elem().Kind() {
			case reflect.Float32:
				f := float32(f)
				rv.Set(reflect.ValueOf(&f))
			case reflect.Float64:
				rv.Set(reflect.ValueOf(&f))
			default:
				rv.Set(reflect.ValueOf(f))
			}
		default:
			rv.Set(reflect.ValueOf(f))
		}
	case SimpleValueFloat64:
		f, err := dec.readFloat64()
		if err != nil {
			return err
		}

		switch rv.Kind() {
		case reflect.Float32:
			rv.SetFloat(float64(f))
		case reflect.Float64:
			rv.SetFloat(f)
		case reflect.Pointer:
			// If the reflect.Value is a pointer, when we can possibly
			// convert it to a float32 or float64.
			switch rv.Type().Elem().Kind() {
			case reflect.Float32:
				f := float32(f)
				rv.Set(reflect.ValueOf(&f))
			case reflect.Float64:
				rv.Set(reflect.ValueOf(&f))
			default:
				rv.Set(reflect.ValueOf(f))
			}
		default:
			rv.Set(reflect.ValueOf(f))
		}
	default:
		return fmt.Errorf("cbor: invalid simple value: %v", ai)
	}
	return nil
}

// decodeUint decodes a CBOR unsigned integer into the given reflect.Value.
func (dec *Decoder) decodeUint(rv reflect.Value, ai byte) error {
	var (
		n   uint64
		err error
	)

	switch ai {
	case 24:
		n, err = dec.readUint8()
	case 25:
		n, err = dec.readUint16()
	case 26:
		n, err = dec.readUint32()
	case 27:
		n, err = dec.readUint64()
	default:
		n = uint64(ai)
	}
	if err != nil {
		return err
	}

	switch rv.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		rv.SetUint(n)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		rv.SetInt(int64(n))
	case reflect.Interface:
		rv.Set(reflect.ValueOf(n))
	case reflect.Ptr:
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		switch rv.Elem().Kind() {
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
			rv.Elem().SetUint(n)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			rv.Elem().SetInt(int64(n))
		case reflect.Interface:
			rv.Elem().Set(reflect.ValueOf(n))
		default:
			return errors.New("cbor: cannot unmarshal uint into " + rv.Type().String())
		}
	default:
		return errors.New("cbor: cannot unmarshal uint into " + rv.Type().String())
	}
	return nil
}

// readUint8 reads an 8-bit unsigned integer from the input stream.
func (dec *Decoder) readUint8() (uint64, error) {
	b, err := dec.readByte()
	return uint64(b), err
}

// readUint16 reads a 16-bit unsigned integer from the input stream.
func (dec *Decoder) readUint16() (uint64, error) {
	var buf [2]byte
	if _, err := io.ReadFull(dec.r, buf[:]); err != nil {
		return 0, err
	}
	return uint64(buf[0])<<8 | uint64(buf[1]), nil
}

// readUint32 reads a 32-bit unsigned integer from the input stream.
func (dec *Decoder) readUint32() (uint64, error) {
	var buf [4]byte
	if _, err := io.ReadFull(dec.r, buf[:]); err != nil {
		return 0, err
	}
	return uint64(buf[0])<<24 | uint64(buf[1])<<16 | uint64(buf[2])<<8 | uint64(buf[3]), nil
}

// readUint64 reads a 64-bit unsigned integer from the input stream.
func (dec *Decoder) readUint64() (uint64, error) {
	var buf [8]byte
	if _, err := io.ReadFull(dec.r, buf[:]); err != nil {
		return 0, err
	}
	return uint64(buf[0])<<56 | uint64(buf[1])<<48 | uint64(buf[2])<<40 | uint64(buf[3])<<32 |
		uint64(buf[4])<<24 | uint64(buf[5])<<16 | uint64(buf[6])<<8 | uint64(buf[7]), nil
}

// decodeInt decodes a CBOR negative integer into the given reflect.Value.
func (dec *Decoder) decodeInt(rv reflect.Value, ai byte) error {
	var n uint64
	nx := &n
	err := dec.decodeUint(reflect.ValueOf(nx), ai)
	if err != nil {
		return err
	}
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		rv.SetInt(-1 - int64(n))
	case reflect.Interface:
		rv.Set(reflect.ValueOf(-1 - int64(n)))
	case reflect.Pointer:
		switch rv.Elem().Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			rv.Elem().SetInt(-1 - int64(n))
		case reflect.Interface:
			rv.Elem().Set(reflect.ValueOf(-1 - int64(n)))
		default:
			return errors.New("cbor: cannot unmarshal int into " + rv.Type().String())
		}
	default:
		return errors.New("cbor: cannot unmarshal int into " + rv.Type().String())
	}
	return nil
}

// decodeBytes decodes a CBOR byte string into the given reflect.Value.
func (dec *Decoder) decodeBytes(rv reflect.Value, ai byte) error {
	var (
		n   uint64
		err error
	)
	switch ai {
	case 24:
		n, err = dec.readUint8()
	case 25:
		n, err = dec.readUint16()
	case 26:
		n, err = dec.readUint32()
	case 27:
		n, err = dec.readUint64()
	default:
		n = uint64(ai)
	}
	if err != nil {
		return err
	}

	if n > math.MaxInt32 {
		return errors.New("cbor: byte string too long")
	}

	if n > uint64(dec.maxBytes) {
		return errors.New("cbor: byte string too long")
	}

	buf := make([]byte, n)
	if _, err := io.ReadFull(dec.r, buf); err != nil {
		return err
	}
	switch rv.Kind() {
	case reflect.Slice:
		if rv.Type().Elem().Kind() != reflect.Uint8 {
			return errors.New("cbor: cannot unmarshal byte string into " + rv.Type().String())
		}
		rv.SetBytes(buf)
	case reflect.Interface:
		rv.Set(reflect.ValueOf(buf))
	default:
		return errors.New("cbor: cannot unmarshal byte string into " + rv.Type().String())
	}
	return nil
}

// decodeString decodes a CBOR text string into the given reflect.Value.
func (dec *Decoder) decodeString(rv reflect.Value, ai byte) error {
	var (
		n   uint64
		err error
	)
	switch ai {
	case 24: // 1-byte uint follows
		n, err = dec.readUint8()
	case 25: // 2-byte uint follows
		n, err = dec.readUint16()
	case 26: // 4-byte uint follows
		n, err = dec.readUint32()
	case 27: // 8-byte uint follows
		n, err = dec.readUint64()
	default: // uint is encoded in initial byte
		n = uint64(ai)
	}
	if err != nil {
		return err
	}
	if n > math.MaxInt32 {
		return errors.New("cbor: string too long")
	}
	// TODO: add a configurable limit to the maximum string length
	buf := make([]byte, n)
	if _, err := io.ReadFull(dec.r, buf); err != nil {
		return err
	}
	switch rv.Kind() {
	case reflect.String:
		rv.SetString(string(buf))
	case reflect.Interface:
		rv.Set(reflect.ValueOf(string(buf)))
	case reflect.Pointer:
		// If we have a pointer to a string, then we can use it. Otherwise, we
		// need to allocate a new string. If it is not a pointer to a string,
		// then we return an error.
		if rv.Type().String() != "*string" {
			return errors.New("cbor: cannot unmarshal string into " + rv.Type().String())
		}
		// If the pointer is nil, then we need to allocate a string.
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv.Elem().SetString(string(buf))
	default:
		return errors.New("cbor: cannot unmarshal string into " + rv.Type().String())
	}
	return nil
}

// decodeArray decodes a CBOR array into the given reflect.Value.
func (dec *Decoder) decodeArray(rv reflect.Value, ai byte) error {
	var (
		n   uint64
		err error
	)
	switch ai {
	case 24: // 1-byte array length
		n, err = dec.readUint8()
	case 25: // 2-byte array length
		n, err = dec.readUint16()
	case 26: // 4-byte array length
		n, err = dec.readUint32()
	case 27: // 8-byte array length
		n, err = dec.readUint64()
	default: // array length is encoded in the initial byte
		n = uint64(ai)
	}
	if err != nil {
		return err
	}

	if n > uint64(dec.maxArrayElements) {
		return errors.New("cbor: array too long")
	}

	switch rv.Kind() {
	case reflect.Slice:
		// If the slice is not nil, we assume it is already the right size.
		//
		// TODO: add a configurable limit to the maximum slice length.
		if rv.IsNil() {

			rv.Set(reflect.MakeSlice(rv.Type(), int(n), int(n)))
		}

		for i := 0; i < int(n); i++ {
			// If the element is not a pointer, we need to get a pointer to it.
			if rv.Type().Elem().Kind() != reflect.Ptr {
				if err := dec.decode(rv.Index(i).Addr()); err != nil {
					return err
				}
			} else {
				if err := dec.decode(rv.Index(i)); err != nil {
					return err
				}
			}
		}
	case reflect.Array:
		if rv.Len() != int(n) {
			return errors.New("cbor: wrong array length")
		}
		for i := 0; i < int(n); i++ {
			// If the element is not a pointer, we need to get a pointer to it.
			if rv.Type().Elem().Kind() != reflect.Ptr {
				if err := dec.decode(rv.Index(i).Addr()); err != nil {
					return err
				}
			} else {
				if err := dec.decode(rv.Index(i)); err != nil {
					return err
				}
			}
		}
	case reflect.Interface:
		s := make([]interface{}, n)
		for i := 0; i < int(n); i++ {
			if err := dec.decode(reflect.ValueOf(&s[i]).Elem()); err != nil {
				return err
			}
		}
		rv.Set(reflect.ValueOf(s))
	default:
		return errors.New("cbor: cannot unmarshal array into " + rv.Type().String())
	}
	return nil
}

// decodeMap decodes a CBOR map into the given reflect.Value.
//
// ai is the additional information byte for the map, which contains the
// number of key/value pairs in the map.
func (dec *Decoder) decodeMap(rv reflect.Value, ai byte) error {
	var (
		n   uint64
		err error
	)
	switch ai {
	case 24:
		n, err = dec.readUint8()
	case 25:
		n, err = dec.readUint16()
	case 26:
		n, err = dec.readUint32()
	case 27:
		n, err = dec.readUint64()
	default:
		n = uint64(ai)
	}
	if err != nil {
		return err
	}

	switch rv.Kind() {
	case reflect.Map:
		// If the map is nil, we need to make it.
		if rv.IsNil() {
			rv.Set(reflect.MakeMap(rv.Type()))
		}
		// Iterate over the key/value pairs in the map based
		// on the determined length (n).
		for i := 0; i < int(n); i++ {
			var key reflect.Value

			// Decode the key.
			switch rv.Type().Key().Kind() {
			case reflect.String:
				key = reflect.New(rv.Type().Key())
				if err := dec.decode(key); err != nil {
					return err
				}
			case reflect.Interface:
				key = reflect.New(rv.Type().Key()).Elem()
				if err := dec.decode(key); err != nil {
					return err
				}
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				key = reflect.New(rv.Type().Key())
				if err := dec.decode(key); err != nil {
					return err
				}
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				key = reflect.New(rv.Type().Key())
				if err := dec.decode(key); err != nil {
					return err
				}
			case reflect.Float32, reflect.Float64:
				key = reflect.New(rv.Type().Key())
				if err := dec.decode(key); err != nil {
					return err
				}
			case reflect.Ptr:
				key = reflect.New(rv.Type().Key().Elem())
				if err := dec.decode(key); err != nil {
					return err
				}
			default:
				return errors.New("cbor: cannot unmarshal map key into " + rv.Type().Key().String())
			}

			// Decode the value.
			switch rv.Type().Elem().Kind() {
			case reflect.String:
				val := reflect.New(rv.Type().Elem())
				if err := dec.decode(val); err != nil {
					return err
				}

				if rv.Type().Key().Kind() != reflect.Ptr {
					key = key.Elem()
				}

				if rv.Type().Elem().Kind() != reflect.Ptr {
					val = val.Elem()
				}

				rv.SetMapIndex(key, val)
			case reflect.Interface:
				var v interface{}
				if err := dec.decode(reflect.ValueOf(&v).Elem()); err != nil {
					return err
				}
				rv.SetMapIndex(key, reflect.ValueOf(v))
			case reflect.Ptr:
				val := reflect.New(rv.Type().Elem().Elem())
				if err := dec.decode(val); err != nil {
					return err
				}

				if rv.Type().Key().Kind() != reflect.Ptr {
					key = key.Elem()
				}

				if rv.Type().Elem().Kind() != reflect.Ptr {
					val = val.Elem()
				}

				rv.SetMapIndex(key, val)
			case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
				val := reflect.New(rv.Type().Elem())
				if err := dec.decode(val); err != nil {
					return err
				}

				// Ensure the key and val is the same type as the map, otherwise
				// we'll get a panic when we try to set the map index.
				//
				// The val starts as an int64, but if the map is an int32
				// we need to convert it to an int32.
				if val.Type().Elem().Kind() != rv.Type().Elem().Kind() {
					val = val.Elem().Convert(rv.Type().Elem())
				}

				if key.Type().Elem().Kind() != rv.Type().Key().Kind() {
					key = key.Elem().Convert(rv.Type().Key())
				}

				if rv.Type().Key().Kind() != reflect.Ptr && key.Kind() == reflect.Ptr {
					key = key.Elem()
				}

				if rv.Type().Elem().Kind() != reflect.Ptr && val.Kind() == reflect.Ptr {
					val = val.Elem()
				}

				rv.SetMapIndex(key, val)
			case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
				val := reflect.New(rv.Type().Elem())
				if err := dec.decode(val); err != nil {
					return err
				}

				if rv.Type().Key().Kind() != reflect.Ptr && key.Kind() == reflect.Ptr {
					key = key.Elem()
				}

				if rv.Type().Elem().Kind() != reflect.Ptr && val.Kind() == reflect.Ptr {
					val = val.Elem()
				}

				rv.SetMapIndex(key, val)
			case reflect.Float32, reflect.Float64:
				val := reflect.New(rv.Type().Elem())
				if err := dec.decode(val); err != nil {
					return err
				}

				if rv.Type().Key().Kind() != reflect.Ptr && key.Kind() == reflect.Ptr {
					key = key.Elem()
				}

				if rv.Type().Elem().Kind() != reflect.Ptr && val.Kind() == reflect.Ptr {
					val = val.Elem()
				}

				rv.SetMapIndex(key, val)
			default:
				val := reflect.New(rv.Type().Elem()).Elem()
				if err := dec.decode(val); err != nil {
					return err
				}

				if rv.Type().Key().Kind() != reflect.Ptr {
					val = val.Elem()
				}

				if rv.Type().Key().Kind() != reflect.Ptr {
					key = key.Elem()
				}

				rv.SetMapIndex(key, val)
			}
		}
	case reflect.Interface:
		m := make(map[interface{}]interface{})
		for i := 0; i < int(n); i++ {
			var key interface{}
			if err := dec.decode(reflect.ValueOf(&key).Elem()); err != nil {
				return err
			}
			var val interface{}
			if err := dec.decode(reflect.ValueOf(&val).Elem()); err != nil {
				return err
			}
			m[key] = val
		}
		rv.Set(reflect.ValueOf(m))
	case reflect.Struct:
		// Structs are treated similarly to maps, but the keys are
		// the struct field names. CBOR map keys can be any type,
		// including string, int, etc. We support all of these
		// types.

		// To reduce allocations, we use a map[int]reflect.Value
		// to cache the field index and value. This is used to
		// avoid the need to call rv.FieldByName for each key.
		fieldCache := make(map[string]reflect.Value, rv.NumField())

		// We need both caches because we need to support both
		// `cbor:"1,keyasint"` and `cbor:"name"` tags.

		// Iterate over the map fields in the struct to build
		// a cache of field names and keyasint values.
		for i := 0; i < rv.NumField(); i++ {
			field := rv.Type().Field(i)

			// If the field is unexported, skip it.
			if field.PkgPath != "" {
				continue
			}

			// If the field has no cbor tag, add it to the
			// field name cache with the field name as the key.
			if field.Tag == "" {
				fieldCache[field.Name] = rv.Field(i)
				continue
			}

			// Check cbor tag for keyasint.
			if tag, ok := field.Tag.Lookup("cbor"); ok {
				// Use index to avoid allocating a new string.
				if idx := strings.Index(tag, ",keyasint"); idx != -1 {
					// If the tag is "keyasint", add it to the field cache.
					fieldCache[tag[:idx]] = rv.Field(field.Index[0])
				} else {
					// If the tag is not "keyasint", add it to the field cache
					// with the tag value as the key.
					fieldCache[tag] = rv.Field(field.Index[0])
				}
			}
		}

		// For each field in the struct, find the corresponding
		// key in the map and decode into the field.
		for i := 0; i < int(n); i++ {
			key, err := dec.readMapKey()
			if err != nil {
				return err
			}

			keyStr := toString(key)

			fv, ok := fieldCache[keyStr]
			if !ok {
				// If the field is not found in the cache, skip it.

				// Read the value and discard it.
				if _, err := dec.readValue(); err != nil {
					return fmt.Errorf("cbor: cannot unmarshal map key into %s: %s", rv.Type().String(), err)
				}

				continue
			}

			// If the field value is not a pointer, we need to create
			// a pointer to the field value and decode into that.
			if fv.Kind() != reflect.Ptr {
				fv = fv.Addr()
			}

			err = dec.decode(fv)
			if err != nil {
				return err
			}
		}
	default:
		return errors.New("cbor: cannot unmarshal map into " + rv.Type().String())
	}
	return nil
}

// decodeTag decodes a CBOR tag into the given reflect.Value.
//
// TODO: add better tag support.
func (dec *Decoder) decodeTag(rv reflect.Value, ai byte) error {
	var (
		n   uint64
		err error
	)
	switch ai {
	case 24:
		n, err = dec.readUint8()
	case 25:
		n, err = dec.readUint16()
	case 26:
		n, err = dec.readUint32()
	case 27:
		n, err = dec.readUint64()
	default:
		n = uint64(ai)
	}
	if err != nil {
		return err
	}
	switch n {
	case 0:
		// RFC 7049, section
		// 2.4.1.  Tag 0:  The Semantic Tag for Big Number
		//
		// The semantic tag 0 is used to indicate that a CBOR data item
		// represents a number that is too big to be represented in the
		// CBOR data item itself.  The number is encoded as a byte string
		// (major type 2), which contains the number's base 2 exponent and
		// coefficient.  The exponent is encoded as an integer (major type
		// 0 or 1), and the coefficient is encoded as an unsigned integer
		// (major type 0).  The coefficient is multiplied by 2 to the power
		// of the exponent to obtain the number's value.  For example, the
		// number 2^1000 is represented as the byte string 0xc4 0x03 0xe8,
		// which in CBOR diagnostic notation is h'c403e8'.
		//
		// The exponent is encoded as a CBOR integer (major type 0 or 1),
		// and the coefficient is encoded as a CBOR unsigned integer
		// (major type 0).  The coefficient is multiplied by 2 to the power
		// of the exponent to obtain the number's value.

		// Read the exponent.
		ai, err := dec.readByte()
		if err != nil {
			return err
		}
		var exp uint64
		switch ai {
		case 24:
			exp, err = dec.readUint8()
		case 25:
			exp, err = dec.readUint16()
		case 26:
			exp, err = dec.readUint32()
		case 27:
			exp, err = dec.readUint64()
		default:
			exp = uint64(ai)
		}
		if err != nil {
			return err
		}

		// Read the coefficient.
		ai, err = dec.readByte()
		if err != nil {
			return err
		}

		// The coefficient is encoded as a CBOR unsigned integer (major
		// type 0).  The coefficient is multiplied by 2 to the power of
		// the exponent to obtain the number's value.
		var coef uint64
		switch ai {
		case 24:
			coef, err = dec.readUint8()
		case 25:
			coef, err = dec.readUint16()
		case 26:
			coef, err = dec.readUint32()
		case 27:
			coef, err = dec.readUint64()
		default:
			coef = uint64(ai)
		}
		if err != nil {
			return err
		}

		// Multiply the coefficient by 2 to the power of the exponent to
		// obtain the number's value.
		val := new(big.Int).Lsh(big.NewInt(int64(coef)), uint(exp))
		rv.Set(reflect.ValueOf(val))
	case 1:
		// RFC 7049, section
		// 2.4.2.  Tag 1:  The Semantic Tag for Decimal Fraction
		//
		// The semantic tag 1 is used to indicate that a CBOR data item
		// represents a decimal fraction.  The number is encoded as an
		// array (major type 4) of two integers.  The first integer is the
		// numerator, and the second integer is the denominator.  For
		// example, the decimal fraction 1/10 is represented as the array
		// [1, 10], which in CBOR diagnostic notation is [1, 10].
		//
		// The numerator and denominator are encoded as CBOR integers
		// (major type 0 or 1).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal decimal fraction into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid decimal fraction")
		}
		num := rv.Index(0)
		den := rv.Index(1)
		if num.Kind() != reflect.Int64 || den.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid decimal fraction")
		}
		rv.Set(reflect.ValueOf(big.NewRat(num.Int(), den.Int())))
	case 2:
		// RFC 7049, section
		// 2.4.3.  Tag 2:  The Semantic Tag for Big Float
		//
		// The semantic tag 2 is used to indicate that a CBOR data item
		// represents a floating-point number that is too big to be
		// represented in the CBOR data item itself.  The number is
		// encoded as an array (major type 4) of two integers.  The first
		// integer is the significand, and the second integer is the
		// base-2 exponent.  For example, the floating-point number
		// 1.234*10^1000 is represented as the array [1234, 1000], which
		// in CBOR diagnostic notation is [1234, 1000].
		//
		// The significand and exponent are encoded as CBOR integers
		// (major type 0 or 1).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal big float into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid big float")
		}
		sig := rv.Index(0)
		exp := rv.Index(1)
		if sig.Kind() != reflect.Int64 || exp.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid big float")
		}

		// convert sig to math big.Float
		sigBf := big.NewFloat(float64(sig.Int()))

		rv.Set(reflect.ValueOf(big.NewFloat(float64(sig.Int())).SetPrec(64).SetMantExp(sigBf, int(exp.Int()))))
	case 3:
		// RFC 7049, section
		// 2.4.4.  Tag 3:  The Semantic Tag for Big Number
		//
		// The semantic tag 3 is used to indicate that a CBOR data item
		// represents a number that is too big to be represented in the
		// CBOR data item itself.  The number is encoded as an array
		// (major type 4) of two integers.  The first integer is the
		// coefficient, and the second integer is the base-2 exponent.
		// For example, the number 2^1000 is represented as the array
		// [2, 1000], which in CBOR diagnostic notation is [2, 1000].
		//
		// The coefficient and exponent are encoded as CBOR integers
		// (major type 0 or 1).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal big number into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid big number")
		}
		coef := rv.Index(0)
		exp := rv.Index(1)
		if coef.Kind() != reflect.Int64 || exp.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid big number")
		}
		rv.Set(reflect.ValueOf(big.NewInt(coef.Int()).Lsh(big.NewInt(coef.Int()), uint(exp.Int()))))
	case 4:
		// RFC 7049, section
		// 2.4.5.  Tag 4:  The Semantic Tag for Big Rational
		//
		// The semantic tag 4 is used to indicate that a CBOR data item
		// represents a rational number that is too big to be represented
		// in the CBOR data item itself.  The number is encoded as an
		// array (major type 4) of two integers.  The first integer is the
		// numerator, and the second integer is the denominator.  For
		// example, the rational number 1/10 is represented as the array
		// [1, 10], which in CBOR diagnostic notation is [1, 10].
		//
		// The numerator and denominator are encoded as CBOR integers
		// (major type 0 or 1).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal big rational into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid big rational")
		}
		num := rv.Index(0)
		den := rv.Index(1)
		if num.Kind() != reflect.Int64 || den.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid big rational")
		}
		rv.Set(reflect.ValueOf(big.NewRat(num.Int(), den.Int())))
	case 5:
		// RFC 7049, section
		// 2.4.6.  Tag 5:  The Semantic Tag for Big Complex
		//
		// The semantic tag 5 is used to indicate that a CBOR data item
		// represents a complex number that is too big to be represented
		// in the CBOR data item itself.  The number is encoded as an
		// array (major type 4) of two arrays.  The first array is the
		// real part, and the second array is the imaginary part.  For
		// example, the complex number 1.234+5.678i is represented as the
		// array [[1, 234], [5, 678]], which in CBOR diagnostic notation
		// is [[1, 234], [5, 678]].
		//
		// The real and imaginary parts are encoded as CBOR arrays
		// (major type 4).
		if err := dec.decode(rv); err != nil {
			return err
		}

		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal big complex into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid big complex")
		}
		real := rv.Index(0)
		imag := rv.Index(1)
		if real.Kind() != reflect.Slice || imag.Kind() != reflect.Slice {
			return errors.New("cbor: invalid big complex")
		}
		if real.Len() != 2 || imag.Len() != 2 {
			return errors.New("cbor: invalid big complex")
		}
		realSig := real.Index(0)
		realExp := real.Index(1)
		imagSig := imag.Index(0)
		imagExp := imag.Index(1)
		if realSig.Kind() != reflect.Int64 || realExp.Kind() != reflect.Int64 || imagSig.Kind() != reflect.Int64 || imagExp.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid big complex")
		}
		// TODO: implement big complex!
		return errors.New("cbor: big complex not fully implemented")
		// rv.Set(reflect.ValueOf(big.NewComplex(big.NewFloat(float64(realSig.Int())).SetPrec(64).SetMantExp(realSig.Int(), int(realExp.Int())), big.NewFloat(float64(imagSig.Int())).SetPrec(64).SetMantExp(imagSig.Int(), int(imagExp.Int())))))
	case 21:
		// RFC 7049, section
		// 2.4.7.  Tag 21:  The Semantic Tag for Decimal Fraction
		//
		// The semantic tag 21 is used to indicate that a CBOR data item
		// represents a decimal fraction.  The number is encoded as an
		// array (major type 4) of two integers.  The first integer is the
		// coefficient, and the second integer is the base-10 exponent.
		// For example, the number 1.234e+5 is represented as the array
		// [1234, 5], which in CBOR diagnostic notation is [1234, 5].
		//
		// The coefficient and exponent are encoded as CBOR integers
		// (major type 0 or 1).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal decimal fraction into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid decimal fraction")
		}
		coef := rv.Index(0)
		exp := rv.Index(1)
		if coef.Kind() != reflect.Int64 || exp.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid decimal fraction")
		}

		// TODO: implement decimal fraction!
		return errors.New("cbor: decimal fraction not fully implemented")
		// rv.Set(reflect.ValueOf(big.NewFloat(float64(coef.Int())).SetPrec(64).SetMantExp(coef.Int(), int(exp.Int()))))
	case 22:
		// RFC 7049, section
		// 2.4.8.  Tag 22:  The Semantic Tag for Big Float
		//
		// The semantic tag 22 is used to indicate that a CBOR data item
		// represents a floating-point number that is too big to be
		// represented in the CBOR data item itself.  The number is
		// encoded as an array (major type 4) of two integers.  The first
		// integer is the significand, and the second integer is the
		// base-2 exponent.  For example, the number 1.234e+5 is
		// represented as the array [1234, 5], which in CBOR diagnostic
		// notation is [1234, 5].
		//
		// The significand and exponent are encoded as CBOR integers
		// (major type 0 or 1).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal big float into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid big float")
		}
		sig := rv.Index(0)
		exp := rv.Index(1)
		if sig.Kind() != reflect.Int64 || exp.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid big float")
		}

		// TODO: implement big float!
		return errors.New("cbor: big float not fully implemented")
		// rv.Set(reflect.ValueOf(big.NewFloat(float64(sig.Int())).SetPrec(64).SetMantExp(sig.Int(), int(exp.Int()))))
	case 23:
		// RFC 7049, section
		// 2.4.9.  Tag 23:  The Semantic Tag for Big Decimal
		//
		// The semantic tag 23 is used to indicate that a CBOR data item
		// represents a decimal number that is too big to be represented
		// in the CBOR data item itself.  The number is encoded as an
		// array (major type 4) of two integers.  The first integer is the
		// coefficient, and the second integer is the base-10 exponent.
		// For example, the number 1.234e+5 is represented as the array
		// [1234, 5], which in CBOR diagnostic notation is [1234, 5].
		//
		// The coefficient and exponent are encoded as CBOR integers
		// (major type 0 or 1).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal big decimal into " + rv.Type().String())
		}
		if rv.Len() != 2 {
			return errors.New("cbor: invalid big decimal")
		}
		coef := rv.Index(0)
		exp := rv.Index(1)
		if coef.Kind() != reflect.Int64 || exp.Kind() != reflect.Int64 {
			return errors.New("cbor: invalid big decimal")
		}

		// TODO: implement big decimal!
		return errors.New("cbor: big decimal not fully implemented")

		// rv.Set(reflect.ValueOf(big.NewFloat(float64(coef.Int())).SetPrec(64).SetMantExp(coef.Int(), int(exp.Int()))))
	case 24:
		// RFC 7049, section
		// 2.4.10.  Tag 24:  The Semantic Tag for URI
		//
		// The semantic tag 24 is used to indicate that a CBOR data item
		// represents a URI.  The URI is encoded as a CBOR text string
		// (major type 3).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.String {
			return errors.New("cbor: cannot unmarshal URI into " + rv.Type().String())
		}
		uri, err := url.Parse(rv.String())
		if err != nil {
			return errors.New("cbor: invalid URI")
		}
		rv.Set(reflect.ValueOf(uri))
	case 25:
		// RFC 7049, section
		// 2.4.11.  Tag 25:  The Semantic Tag for Base64URL
		//
		// The semantic tag 25 is used to indicate that a CBOR data item
		// represents a sequence of octets that is encoded in base64url
		// [RFC4648].  The data item is encoded as a CBOR text string
		// (major type 3).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.String {
			return errors.New("cbor: cannot unmarshal base64url into " + rv.Type().String())
		}
		b, err := base64.URLEncoding.DecodeString(rv.String())
		if err != nil {
			return errors.New("cbor: invalid base64url")
		}
		rv.Set(reflect.ValueOf(b))
	case 26:
		// RFC 7049, section
		// 2.4.12.  Tag 26:  The Semantic Tag for Base64
		//
		// The semantic tag 26 is used to indicate that a CBOR data item
		// represents a sequence of octets that is encoded in base64
		// [RFC4648].  The data item is encoded as a CBOR text string
		// (major type 3).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.String {
			return errors.New("cbor: cannot unmarshal base64 into " + rv.Type().String())
		}
		b, err := base64.StdEncoding.DecodeString(rv.String())
		if err != nil {
			return errors.New("cbor: invalid base64")
		}
		rv.Set(reflect.ValueOf(b))
	case 27:
		// RFC 7049, section
		// 2.4.13.  Tag 27:  The Semantic Tag for Regular Expression
		//
		// The semantic tag 27 is used to indicate that a CBOR data item
		// represents a regular expression.  The regular expression is
		// encoded as a CBOR text string (major type 3).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.String {
			return errors.New("cbor: cannot unmarshal regular expression into " + rv.Type().String())
		}
		re, err := regexp.Compile(rv.String())
		if err != nil {
			return errors.New("cbor: invalid regular expression")
		}
		rv.Set(reflect.ValueOf(re))
	case 28:
		// RFC 7049, section
		// 2.4.14.  Tag 28:  The Semantic Tag for MIME Message
		//
		// The semantic tag 28 is used to indicate that a CBOR data item
		// represents a MIME message.  The MIME message is encoded as a
		// CBOR text string (major type 3).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.String {
			return errors.New("cbor: cannot unmarshal MIME message into " + rv.Type().String())
		}
		mime, err := mail.ReadMessage(strings.NewReader(rv.String()))
		if err != nil {
			return errors.New("cbor: invalid MIME message")
		}
		rv.Set(reflect.ValueOf(mime))
	case 29:
		// RFC 7049, section
		// 2.4.15.  Tag 29:  The Semantic Tag for CBOR Sequence
		//
		// The semantic tag 29 is used to indicate that a CBOR data item
		// represents a CBOR sequence.  The CBOR sequence is encoded as a
		// CBOR array (major type 4).
		if err := dec.decode(rv); err != nil {
			return err
		}
		if rv.Kind() != reflect.Slice {
			return errors.New("cbor: cannot unmarshal CBOR sequence into " + rv.Type().String())
		}
	default:
		return errors.New("cbor: unknown tag " + strconv.Itoa(int(n)))
	}
	return nil
}

// decode decodes a CBOR value into rv. rv must be a pointer to a value,
// or an interface value.
func (dec *Decoder) decode(rv reflect.Value) error {
	if rv.Kind() != reflect.Ptr {
		return errors.New("cbor: cannot unmarshal into non-pointer " + rv.Type().String())
	}
	if rv.IsNil() {
		rv.Set(reflect.New(rv.Type().Elem()))
	}
	rv = rv.Elem()
	if rv.Kind() == reflect.Interface {
		if rv.NumMethod() != 0 {
			return errors.New("cbor: cannot unmarshal into non-empty interface " + rv.Type().String())
		}
		rv.Set(reflect.ValueOf(&dec.r).Elem())
		return nil
	}
	if rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}
	if rv.Kind() == reflect.Struct {
		return dec.decodeStruct(rv)
	}
	if rv.Kind() == reflect.Slice {
		return dec.decodeSlice(rv)
	}
	if rv.Kind() == reflect.Map {
		return dec.decodeMap(rv, byte(rv.Len())) // TODO: is this correct "ai" value for map?
	}
	return dec.decodeBasic(rv)
}

// decodeStruct decodes a CBOR map into rv. rv must be a pointer to a struct.
//
// CBOR structs are represented as CBOR maps. The keys of the map are
// the names of the struct fields. The values of the map are the values
// of the struct fields.
func (dec *Decoder) decodeStruct(rv reflect.Value) error {
	// Read the map header, n is the number of key/value pairs.
	n, err := dec.readMapHeader()
	if err != nil {
		return err
	}

	for i := 0; i < n; i++ {
		key, err := dec.readString()
		if err != nil {
			return err
		}

		fv := rv.FieldByNameFunc(func(name string) bool {
			return strings.EqualFold(name, key)
		})

		if !fv.IsValid() {
			return errors.New("cbor: unknown field " + key)
		}

		if err := dec.decode(fv.Addr()); err != nil {
			return err
		}
	}

	return nil
}

// decodeSlice decodes a CBOR array into rv. rv must be a pointer to a slice.
func (dec *Decoder) decodeSlice(rv reflect.Value) error {
	n, err := dec.readArrayHeader()
	if err != nil {
		return err
	}
	// TODO: add limit.

	// Allocate a new slice.
	sv := reflect.MakeSlice(rv.Type(), n, n)
	// Read the slice elements.
	for i := 0; i < n; i++ {
		if err := dec.decode(sv.Index(i).Addr()); err != nil {
			return err
		}
	}
	rv.Set(sv)
	return nil
}

// decodeBasic decodes a CBOR value into rv. rv must be a pointer to a basic
// value.
func (dec *Decoder) decodeBasic(rv reflect.Value) error {
	switch rv.Kind() {
	case reflect.Bool:
		b, err := dec.readBool()
		if err != nil {
			return err
		}
		rv.SetBool(b)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := dec.readInt()
		if err != nil {
			return err
		}
		rv.SetInt(int64(n))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := dec.readUint()
		if err != nil {
			return err
		}
		rv.SetUint(uint64(n))
	case reflect.Float32, reflect.Float64:
		f, err := dec.readFloat()
		if err != nil {
			return err
		}
		rv.SetFloat(f)
	case reflect.String:
		s, err := dec.readString()
		if err != nil {
			return err
		}
		rv.SetString(s)
	case reflect.Interface:
		if rv.NumMethod() != 0 {
			return errors.New("cbor: cannot unmarshal into non-empty interface " + rv.Type().String())
		}
		rv.Set(reflect.ValueOf(&dec.r).Elem())
	default:
		return errors.New("cbor: cannot unmarshal into " + rv.Type().String())
	}
	return nil
}

// readArrayHeader reads an array header from the CBOR stream.
func (dec *Decoder) readArrayHeader() (int, error) {
	b, err := dec.readByte()
	if err != nil {
		return 0, err
	}
	switch {
	case b >= 0x80 && b <= 0x8f:
		return int(b & 0x0f), nil
	case b == 0x9f:
		return dec.readInt()
	case b >= 0x40 && b <= 0x5f: // handle []byte
		n := int(b & 0x1f)
		return n, nil
	default:
		return 0, fmt.Errorf("cbor: invalid array header: %X", b)
	}
}

// readMapHeader reads a map header from the CBOR stream.
func (dec *Decoder) readMapHeader() (int, error) {
	b, err := dec.readByte()
	if err != nil {
		return 0, err
	}
	switch {
	case b >= 0xa0 && b <= 0xaf:
		return int(b & 0x0f), nil
	case b == 0xbf:
		return dec.readInt()
	default:
		return 0, errors.New("cbor: invalid map header")
	}
}

// readBool reads a boolean value from the CBOR stream.
func (dec *Decoder) readBool() (bool, error) {
	b, err := dec.readByte()
	if err != nil {
		return false, err
	}
	switch b {
	case 0xf4:
		return false, nil
	case 0xf5:
		return true, nil
	default:
		return false, errors.New("cbor: invalid boolean value")
	}
}

// readInt reads an integer value from the CBOR stream.
func (dec *Decoder) readInt() (int, error) {
	b, err := dec.readByte()
	if err != nil {
		return 0, err
	}

	// Check if unsigned 32-bit integer.
	if b == 0x1A {
		n, err := dec.readUint32()
		if err != nil {
			return 0, err
		}
		return int(n), nil
	}

	// Check if unsigned 64-bit integer.
	if b == 0x1B {
		n, err := dec.readUint64()
		if err != nil {
			return 0, err
		}
		return int(n), nil
	}

	switch {
	case b <= 0x17:
		return int(b), nil
	case b >= 0x18 && b <= 0x1f:
		return int(b & 0x1f), nil
	case b == 0x20:
		n, err := dec.readUint64()
		if err != nil {
			return 0, err
		}
		return int(n), nil
	case b == 0x21:
		n, err := dec.readUint32()
		if err != nil {
			return 0, err
		}
		return int(n), nil
	case b == 0x22:
		n, err := dec.readUint64()
		if err != nil {
			return 0, err
		}
		return int(n), nil
	default:
		return 0, errors.New("cbor: invalid integer value: " + fmt.Sprintf("%X", b))
	}
}

// readUint reads an unsigned integer value from the CBOR stream.
func (dec *Decoder) readUint() (uint, error) {
	b, err := dec.readByte()
	if err != nil {
		return 0, err
	}
	switch {
	case b <= 0x17:
		return uint(b), nil
	case b >= 0x18 && b <= 0x1f:
		return uint(b & 0x1f), nil
	case b == 0x20:
		n, err := dec.readUint16()
		if err != nil {
			return 0, err
		}
		return uint(n), nil
	case b == 0x21:
		n, err := dec.readUint32()
		if err != nil {
			return 0, err
		}
		return uint(n), nil
	case b == 0x22:
		n, err := dec.readUint64()
		if err != nil {
			return 0, err
		}
		return uint(n), nil
	default:
		return uint(b), nil
		// return 0, errors.New("cbor: invalid usigned integer value: " + fmt.Sprintf("%X", b))
	}
}

// readFloat reads a floating point value from the CBOR stream.
func (dec *Decoder) readFloat() (float64, error) {
	b, err := dec.readByte()
	if err != nil {
		return 0, err
	}
	switch b {
	case 0xf9:
		return dec.readFloat16()
	case 0xfa:
		return dec.readFloat32()
	case 0xfb:
		return dec.readFloat64()
	default:
		// If the value is not a float, then it must be an integer.
		// We can convert it to a float by casting it to a float64.
		// This is a bit of a hack, but it works.
		return float64(b), nil
	}
}

// readFloat16 reads a 16-bit floating point value from the CBOR stream.
func (dec *Decoder) readFloat16() (float64, error) {
	b, err := dec.readUint16()
	if err != nil {
		return 0, err
	}
	return float64(math.Float32frombits(uint32(b))), nil
}

// readFloat32 reads a 32-bit floating point value from the CBOR stream.
func (dec *Decoder) readFloat32() (float64, error) {
	b, err := dec.readUint32()
	if err != nil {
		return 0, err
	}
	return float64(math.Float32frombits(uint32(b))), nil
}

// readFloat64 reads a 64-bit floating point value from the CBOR stream.
func (dec *Decoder) readFloat64() (float64, error) {
	b, err := dec.readUint64()
	if err != nil {
		return 0, err
	}
	return math.Float64frombits(b), nil
}

// readString reads a string value from the CBOR stream.
func (dec *Decoder) readString() (string, error) {
	b, err := dec.readByte()
	if err != nil {
		return "", err
	}
	switch {
	case b >= 0x60 && b <= 0x77: // less than 24 bytes
		n := int(b & 0x1f)

		return dec.readStringBytes(n)
	case b >= 0x78 && b <= 0x7f: // more than 24 bytes (less than 256 bytes)
		n, err := dec.readInt()
		if err != nil {
			return "", err
		}
		return dec.readStringBytes(n)
	case b == 0x7f: // indefinite length
		n, err := dec.readInt()
		if err != nil {
			return "", err
		}
		return dec.readStringBytes(n)
	case b == 0xf6: // null string
		return "", nil
	default:
		return "", fmt.Errorf("cbor: invalid string value: %X", b)
	}
}

// readStringBytes reads a string value from the CBOR stream.
func (dec *Decoder) readStringBytes(n int) (string, error) {
	if n == 0 {
		return "", nil
	}

	if n > dec.maxStringBytes {
		return "", fmt.Errorf("cbor: string too large: %d bytes", n)
	}

	buf := make([]byte, n)
	_, err := io.ReadFull(dec.r, buf)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// readMapKey reads a map key from the CBOR stream.
//
// Used internally by decodeMap for decoding struct fields.
func (dec *Decoder) readMapKey() (any, error) {
	b, err := dec.readByte()
	if err != nil {
		return nil, err
	}
	switch {
	case b <= 0x17:
		return int(b), nil
	case b >= 0x18 && b <= 0x1f:
		return int(b & 0x1f), nil
	case b == 0x20:
		n, err := dec.readUint16()
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case b == 0x21:
		n, err := dec.readUint32()
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case b == 0x22:
		n, err := dec.readUint64()
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case b >= 0x30 && b <= 0x37: // less than 24 bytes
		n := int(b & 0x1f)

		return dec.readStringBytes(n)
	case b >= 0x38 && b <= 0x3f: // more than 24 bytes (less than 256 bytes)
		n := int(b & 0x1f)

		return dec.readStringBytes(n)
	case b == 0x7f: // indefinite length
		n, err := dec.readInt()
		if err != nil {
			return nil, err
		}
		return dec.readStringBytes(n)
	case b == 0x3f: // more than 256 bytes (less than 65536 bytes)
		n, err := dec.readInt()
		if err != nil {
			return nil, err
		}
		return dec.readStringBytes(n)
	case b == 0xf6: // null string
		return "", nil
	case b == 0xf7: // null bytes
		return []byte{}, nil
	case b == 0xf8: // null array
		return []any{}, nil
	case b == 0xf9: // null map
		return map[any]any{}, nil
	case b == 0xfa: // null tag
		return nil, nil
	case b == 0xfb: // null simple value
		return nil, nil
	case b == 0xff: // null
		return nil, nil
	case b == 0x40:
		return false, nil
	case b >= 0x60 && b <= 0x77: // less than 24 bytes
		n := int(b & 0x1f)

		return dec.readStringBytes(n)
	case b >= 0x78 && b <= 0x7f: // more than 24 bytes (less than 256 bytes)
		n, err := dec.readInt()
		if err != nil {
			return nil, err
		}
		return dec.readStringBytes(n)
	case b == 0x7f: // indefinite length
		n, err := dec.readInt()
		if err != nil {
			return nil, err
		}
		return dec.readStringBytes(n)
	default:
		return nil, fmt.Errorf("cbor: invalid map key: %X", b)
	}
}

// toString converts any Go value to a string as fast as possible
// while avoiding allocations.
func toString(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	case int:
		return strconv.Itoa(v)
	case int8:
		return strconv.Itoa(int(v))
	case int16:
		return strconv.Itoa(int(v))
	case int32:
		return strconv.Itoa(int(v))
	case int64:
		return strconv.Itoa(int(v))
	case uint:
		return strconv.Itoa(int(v))
	case uint8:
		return strconv.Itoa(int(v))
	case uint16:
		return strconv.Itoa(int(v))
	case uint32:
		return strconv.Itoa(int(v))
	case uint64:
		return strconv.Itoa(int(v))
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		if v {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		return fmt.Sprintf("%v", v)
	}
}

// readValue reads a value from the CBOR stream.
func (dec *Decoder) readValue() (any, error) {
	b, err := dec.readByte()
	if err != nil {
		return nil, err
	}
	switch {
	case b <= 0x17:
		return int(b), nil
	case b >= 0x18 && b <= 0x1f:
		return int(b & 0x1f), nil
	case b == 0x20:
		n, err := dec.readUint16()
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case b == 0x21:
		n, err := dec.readUint32()
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case b == 0x22:
		n, err := dec.readUint64()
		if err != nil {
			return nil, err
		}
		return int(n), nil
	case b >= 0x30 && b <= 0x37:
		n := int(b & 0x1f)

		return dec.readStringBytes(n)
	case b >= 0x38 && b <= 0x3f:
		n := int(b & 0x1f)

		return dec.readStringBytes(n)
	case b == 0x3f:
		n, err := dec.readInt()
		if err != nil {
			return nil, err
		}
		return dec.readStringBytes(n)
	default:
		return nil, fmt.Errorf("cbor: invalid value: %X", b)
	}
}
