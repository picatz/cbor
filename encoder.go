package cbor

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"reflect"
)

// Encoder is a minimal CBOR encoder.
type Encoder struct {
	// contains filtered or unexported fields
	w io.Writer
}

// NewEncoder returns a new encoder that writes to w.
func NewEncoder(w io.Writer) *Encoder {
	return &Encoder{w: w}
}

// Encode writes the CBOR encoding of v to the stream.
func (e *Encoder) Encode(v interface{}) error {
	rv := reflect.ValueOf(v)

	// Handle nil.
	if !rv.IsValid() {
		return e.writeNull()
	}

	// Handle types.
	switch rv.Kind() {
	case reflect.Bool:
		return e.writeBool(rv.Bool())
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return e.writeInt(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return e.writeUint(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return e.writeFloat(rv.Float())
	case reflect.String:
		return e.writeString(rv.String())
	case reflect.Array, reflect.Slice:
		return e.writeArray(rv)
	case reflect.Map:
		return e.writeMap(rv)
	case reflect.Struct:
		return e.writeStruct(rv)
	}

	return fmt.Errorf("cbor: unsupported type: %T", v)
}

// writeNull writes a null value.
func (e *Encoder) writeNull() error {
	_, err := e.w.Write([]byte{0xf6})
	return err
}

// writeBool writes a boolean value.
func (e *Encoder) writeBool(v bool) error {
	if v {
		_, err := e.w.Write([]byte{0xf5})
		return err
	}
	_, err := e.w.Write([]byte{0xf4})
	return err
}

// writeInt writes an integer value.
func (e *Encoder) writeInt(v int64) error {
	switch {
	case v >= 0 && v <= 23:
		_, err := e.w.Write([]byte{byte(v)})
		return err
	case v >= 24 && v <= 255:
		_, err := e.w.Write([]byte{0x18, byte(v)})
		return err
	case v >= 256 && v <= 65535:
		_, err := e.w.Write([]byte{0x19, byte(v >> 8), byte(v)})
		return err
	case v >= 65536 && v <= 4294967295:
		_, err := e.w.Write([]byte{0x1a, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
		return err
	case v >= 4294967296 && v <= math.MaxInt64-1:
		_, err := e.w.Write([]byte{0x1b, byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
		return err
	}
	return fmt.Errorf("cbor: integer out of range: %d", v)
}

// writeUint writes an unsigned integer value.
func (e *Encoder) writeUint(v uint64) error {
	switch {
	case v <= 23:
		_, err := e.w.Write([]byte{byte(v)})
		return err
	case v >= 24 && v <= 255:
		_, err := e.w.Write([]byte{0x18, byte(v)})
		return err
	case v >= 256 && v <= 65535:
		_, err := e.w.Write([]byte{0x19, byte(v >> 8), byte(v)})
		return err
	case v >= 65536 && v <= 4294967295:
		_, err := e.w.Write([]byte{0x1a, byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
		return err
	case v >= 4294967296 && v <= math.MaxUint64-1:
		_, err := e.w.Write([]byte{0x1b, byte(v >> 56), byte(v >> 48), byte(v >> 40), byte(v >> 32), byte(v >> 24), byte(v >> 16), byte(v >> 8), byte(v)})
		return err
	}
	return fmt.Errorf("cbor: integer out of range: %d", v)
}

// writeFloat writes a floating point value.
func (e *Encoder) writeFloat(v float64) error {
	// Encode as a 64-bit float.
	_, err := e.w.Write([]byte{0xfb})
	if err != nil {
		return err
	}

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], math.Float64bits(v))
	_, err = e.w.Write(buf[:])
	return err
}

// writeString writes a string value.
func (e *Encoder) writeString(v string) error {
	// Encode as a test string
	_, err := e.w.Write([]byte{
		0x78, // text string
		byte(len(v)),
	})

	if err != nil {
		return err
	}

	_, err = e.w.Write([]byte(v))
	return err
}

// writeArray writes an array value.
func (e *Encoder) writeArray(v reflect.Value) error {
	// Encode as an array.
	_, err := e.w.Write([]byte{
		0x98,
		byte(v.Len()),
	})

	if err != nil {
		return err
	}

	for i := 0; i < v.Len(); i++ {
		if err := e.Encode(v.Index(i).Interface()); err != nil {
			return err
		}
	}

	return nil
}

// writeMap writes a map value.
func (e *Encoder) writeMap(v reflect.Value) error {
	// Encode as a map.
	_, err := e.w.Write([]byte{
		0xb8,
		byte(v.Len()),
	})

	if err != nil {
		return err
	}

	// getKey returns the human friendly key type
	// to encode the map key.
	getKey := func(key reflect.Value) interface{} {
		switch key.Kind() {
		case reflect.String:
			return key.String()
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return key.Int()
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return key.Uint()
		default:
			return key.Interface()
		}
	}

	for _, key := range v.MapKeys() {
		// Encode key, then value.
		if err := e.Encode(getKey(key)); err != nil {
			return err
		}

		if err := e.Encode(v.MapIndex(key).Interface()); err != nil {
			return err
		}
	}

	return nil
}

// writeStruct writes a struct value.
func (e *Encoder) writeStruct(v reflect.Value) error {
	// Encode as a map.
	_, err := e.w.Write([]byte{
		0xb8,
		byte(v.NumField()),
	})

	if err != nil {
		return err
	}

	for i := 0; i < v.NumField(); i++ {
		if err := e.Encode(v.Field(i).Interface()); err != nil {
			return err
		}
	}

	return nil
}
