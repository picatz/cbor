package cbor

// MajorType is the major type of a CBOR item: the high three bits of its
// initial byte.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.1
type MajorType int

// Major types defined in RFC 8949.
const (
	MajorTypeUnsignedInt MajorType = 0 // unsigned integer
	MajorTypeNegativeInt MajorType = 1 // negative integer
	MajorTypeByteString  MajorType = 2 // byte string
	MajorTypeTextString  MajorType = 3 // UTF-8 text string
	MajorTypeArray       MajorType = 4 // array
	MajorTypeMap         MajorType = 5 // map
	MajorTypeTag         MajorType = 6 // tagged item
	MajorTypeSimple      MajorType = 7 // simple value or float
)

// SimpleValue is a CBOR simple value. Undefined and unassigned simple values
// decode as SimpleValue; values 24 through 31 cannot be encoded.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.3
type SimpleValue int

// Simple values defined in RFC 8949.
const (
	SimpleValueFalse     SimpleValue = 20
	SimpleValueTrue      SimpleValue = 21
	SimpleValueNull      SimpleValue = 22
	SimpleValueUndefined SimpleValue = 23
)

// Tag numbers defined in RFC 8949 Table 5, for use with Tag and TaggedValue.
// This package preserves tags without interpreting their content.
//
// https://www.rfc-editor.org/rfc/rfc8949.html#section-3.4
const (
	TagDateTimeString    uint64 = 0     // RFC 3339 date/time text
	TagEpochTime         uint64 = 1     // seconds since the epoch, integer or float
	TagUnsignedBignum    uint64 = 2     // unsigned bignum byte string
	TagNegativeBignum    uint64 = 3     // negative bignum byte string
	TagDecimalFraction   uint64 = 4     // [exponent, mantissa] base 10
	TagBigfloat          uint64 = 5     // [exponent, mantissa] base 2
	TagExpectBase64URL   uint64 = 21    // expected conversion to base64url
	TagExpectBase64      uint64 = 22    // expected conversion to base64
	TagExpectBase16      uint64 = 23    // expected conversion to base16
	TagEncodedCBOR       uint64 = 24    // byte string holding an encoded item
	TagURI               uint64 = 32    // URI text
	TagBase64URL         uint64 = 33    // base64url text
	TagBase64            uint64 = 34    // base64 text
	TagMIMEMessage       uint64 = 36    // MIME message text
	TagSelfDescribedCBOR uint64 = 55799 // self-described CBOR marker
)
