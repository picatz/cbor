package cbor

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

// SimpleValue is a CBOR simple value. Undefined and unassigned simple values
// decode as SimpleValue; values 24 through 31 cannot be encoded.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.3
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
