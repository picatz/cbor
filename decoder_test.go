package cbor_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/picatz/cbor"
)

func ExampleDecoder() {
	const cborStream = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64" // {"hello": "world"}

	var value map[string]string
	err := cbor.NewDecoder(bytes.NewBufferString(cborStream)).Decode(&value)
	if err != nil {
		panic(err)
	}

	// Output: world
	fmt.Println(value["hello"])
}

func TestDecodeInt(t *testing.T) {
	data := "\x01"

	var value int
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != 1 {
		t.Fatal("expected 1, got", value)
	}
}

func TestDecodeIntPointer(t *testing.T) {
	data := "\x01"

	var value *int
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != 1 {
		t.Fatal("expected 1, got", value)
	}
}

func TestDecodeInt64(t *testing.T) {
	data := "\x01"

	var value int64
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != 1 {
		t.Fatal("expected 1, got", value)
	}
}

func TestDecodeInt64Pointer(t *testing.T) {
	data := "\x01"

	var value *int64
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != 1 {
		t.Fatal("expected 1, got", value)
	}
}

func TestDecodeFloat64(t *testing.T) {
	data := "\xFB\x40\x09\x1E\xB8\x51\xEB\x85\x1F" // 3.14

	var value float64
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != 3.14 {
		t.Fatal("expected 3.14, got", value)
	}
}

func TestDecodeFloat64Pointer(t *testing.T) {
	data := "\xFB\x40\x09\x1E\xB8\x51\xEB\x85\x1F" // 3.14

	var value *float64
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != 3.14 {
		t.Fatal("expected 3.14, got", value)
	}
}

func TestDecodeFloat32(t *testing.T) {
	data := "\xFA\x40\x48\xF5\xC3" // 3.14

	var value float32
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != 3.14 {
		t.Fatal("expected 3.14, got", value)
	}
}

func TestDecodeFloat32Pointer(t *testing.T) {
	data := "\xFA\x40\x48\xF5\xC3" // 3.14

	var value *float32
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != 3.14 {
		t.Fatal("expected 3.14, got", value)
	}
}

func TestDecodeBool(t *testing.T) {
	data := "\xF5" // true

	var value bool
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != true {
		t.Fatal("expected true")
	}
}

func TestDecodeBoolPointer(t *testing.T) {
	data := "\xF5" // true

	var value *bool
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != true {
		t.Fatal("expected true")
	}
}

func TestDecodeString(t *testing.T) {
	data := "\x66\x66\x6F\x6F\x62\x61\x72" // "foobar"

	var value string
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != "foobar" {
		t.Fatal("expected foobar")
	}
}

func TestDecodeStringPointer(t *testing.T) {
	data := "\x66\x66\x6F\x6F\x62\x61\x72" // "foobar"

	var value *string
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != "foobar" {
		t.Fatal("expected foobar")
	}
}

func TestDecodeBytes(t *testing.T) {
	data := "\x46\x66\x6F\x6F\x62\x61\x72" // "foobar"

	var value []byte
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if string(value) != "foobar" {
		t.Fatal("expected foobar")
	}
}

func TestDecodeNil(t *testing.T) {
	data := "\xF6" // nil

	var value interface{}
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != nil {
		t.Fatal("expected nil")
	}
}

func TestDecodeArray(t *testing.T) {
	t.Run("floats", func(t *testing.T) {
		data := "\x82\xFB\x40\x09\x1E\xB8\x51\xEB\x85\x1F\xFB\x40\x19\x1E\xB8\x51\xEB\x85\x1F" // [3.14, 6.28]

		t.Run("pointer", func(t *testing.T) {
			var value []*float64
			err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
			if err != nil {
				t.Fatal(err)
			}

			if len(value) != 2 {
				t.Fatal("expected 2, got", len(value))
			}
			if *value[0] != 3.14 {
				t.Fatal("expected 3.14, got", value[0])
			}
			if *value[1] != 6.28 {
				t.Fatal("expected 6.28, got", value[1])
			}
		})

		t.Run("non-pointer", func(t *testing.T) {
			var value []float64
			err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
			if err != nil {
				t.Fatal(err)
			}

			if len(value) != 2 {
				t.Fatal("expected 2, got", len(value))
			}
			if value[0] != 3.14 {
				t.Fatal("expected 3.14, got", value[0])
			}
			if value[1] != 6.28 {
				t.Fatal("expected 6.28, got", value[1])
			}
		})
	})

	t.Run("ints", func(t *testing.T) {
		data := "\x82\x01\x02" // [1, 2]

		t.Run("pointer", func(t *testing.T) {
			var value []*int
			err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
			if err != nil {
				t.Fatal(err)
			}

			if len(value) != 2 {
				t.Fatal("expected 2, got", len(value))
			}
			if *value[0] != 1 {
				t.Fatal("expected 1, got", value[0])
			}
			if *value[1] != 2 {
				t.Fatal("expected 2, got", value[1])
			}
		})

		t.Run("non-pointer", func(t *testing.T) {
			var value []int
			err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
			if err != nil {
				t.Fatal(err)
			}

			if len(value) != 2 {
				t.Fatal("expected 2, got", len(value))
			}
			if value[0] != 1 {
				t.Fatal("expected 1, got", value[0])
			}
			if value[1] != 2 {
				t.Fatal("expected 2, got", value[1])
			}
		})
	})

	t.Run("strings", func(t *testing.T) {
		data := "\x82\x63\x66\x6F\x6F\x63\x62\x61\x72" // ["foo", "bar"]

		t.Run("pointer", func(t *testing.T) {
			var value []*string
			err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
			if err != nil {
				t.Fatal(err)
			}

			if len(value) != 2 {
				t.Fatal("expected 2, got", len(value))
			}
			if *value[0] != "foo" {
				t.Fatal("expected foo, got", value[0])
			}
			if *value[1] != "bar" {
				t.Fatal("expected bar, got", value[1])
			}
		})

		t.Run("non-pointer", func(t *testing.T) {
			var value []string
			err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
			if err != nil {
				t.Fatal(err)
			}

			if len(value) != 2 {
				t.Fatal("expected 2, got", len(value))
			}
			if value[0] != "foo" {
				t.Fatal("expected foo, got", value[0])
			}
			if value[1] != "bar" {
				t.Fatal("expected bar, got", value[1])
			}
		})
	})

	t.Run("structs", func(t *testing.T) {
		t.Run("full", func(t *testing.T) {
			data := "\x81\xA2\x63\x66\x6F\x6F\x63\x62\x61\x72\x63\x62\x61\x7A\xF6" // [{"foo":"bar","baz":null}]

			t.Run("pointer", func(t *testing.T) {
				var value []*struct {
					Foo string
					Baz *string
				}
				err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
				if err != nil {
					t.Fatal(err)
				}

				if value[0].Foo != "bar" {
					t.Fatal("expected bar, got", value[0].Foo)
				}

				// Note: it's super weird to handle this case.
				//
				// If the CBOR map value is null, how should
				// the struct field be handled? Should it be
				// set to nil, or should it be set to the
				// zero value of the field type? How do you distinguish
				// between a null value, zero value, or a non-existent
				// value in the data?
				//
				// TODO: figure out how to handle this case canonically,
				//       and document it.
				if value[0].Baz == nil {
					t.Fatal("expected non-nil, got nil")
				}
				if *value[0].Baz != "" {
					t.Fatal("expected empty string, got", *value[0].Baz)
				}
			})

			t.Run("non-pointer", func(t *testing.T) {
				var value []struct {
					Foo string
					Baz *string
				}
				err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
				if err != nil {
					t.Fatal(err)
				}

				if value[0].Foo != "bar" {
					t.Fatal("expected bar, got", value[0].Foo)
				}
				if value[0].Baz == nil {
					t.Fatal("expected non-nil, got nil")
				}
				if *value[0].Baz != "" {
					t.Fatal("expected empty string, got", *value[0].Baz)
				}
			})
		})

		t.Run("short", func(t *testing.T) {
			data := "\x81\xA2\x63\x66\x6F\x6F\x63\x62\x61\x72" // [{"foo":"bar"}]

			t.Run("pointer", func(t *testing.T) {
				var value []*struct {
					Foo string
					Baz *string
				}
				err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
				if err != nil {
					t.Fatal(err)
				}

				if value[0].Foo != "bar" {
					t.Fatal("expected bar, got", value[0].Foo)
				}
				if value[0].Baz != nil {
					t.Fatal("expected nil, got", value[0].Baz)
				}
			})

			t.Run("non-pointer", func(t *testing.T) {
				var value []struct {
					Foo string
					Baz *string
				}
				err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
				if err != nil {
					t.Fatal(err)
				}

				if value[0].Foo != "bar" {
					t.Fatal("expected bar, got", value[0].Foo)
				}
				if value[0].Baz != nil {
					t.Fatal("expected nil, got", value[0].Baz)
				}
			})
		})
	})
}

func TestDecodeMap(t *testing.T) {
	t.Run("string keys and values", func(t *testing.T) {
		data := "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64" // {"hello":"world"}

		var value map[string]string
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 1 {
			t.Fatal("expected 1, got", len(value))
		}
		if value["hello"] != "world" {
			t.Fatal("expected world, got", value["hello"])
		}
	})

	t.Run("string pointer keys and values", func(t *testing.T) {
		data := "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64"

		var value map[*string]*string
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 1 {
			t.Fatal("expected 1, got", len(value))
		}

		for k, v := range value {
			if *k != "hello" {
				t.Fatal("expected hello, got", *k)
			}
			if *v != "world" {
				t.Fatal("expected world, got", *v)
			}
		}
	})

	t.Run("string pointer keys and non-pointer values", func(t *testing.T) {
		data := "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64"

		var value map[*string]string
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 1 {
			t.Fatal("expected 1, got", len(value))
		}

		for k, v := range value {
			if *k != "hello" {
				t.Fatal("expected hello, got", *k)
			}
			if v != "world" {
				t.Fatal("expected world, got", v)
			}
		}
	})

	t.Run("string non-pointer keys and pointer values", func(t *testing.T) {
		data := "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64"

		var value map[string]*string
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 1 {
			t.Fatal("expected 1, got", len(value))
		}

		for k, v := range value {
			if k != "hello" {
				t.Fatal("expected hello, got", k)
			}
			if *v != "world" {
				t.Fatal("expected world, got", *v)
			}
		}
	})

	t.Run("int keys and values", func(t *testing.T) {
		data := "\xA2\x01\x02\x03\x04" // {1: 2, 3: 4}

		var value map[int]int
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		if value[1] != 2 {
			t.Fatal("expected 2, got", value[1])
		}
		if value[3] != 4 {
			t.Fatal("expected 4, got", value[3])
		}
	})

	t.Run("int pointer keys and values", func(t *testing.T) {
		data := "\xA2\x01\x02\x03\x04" // {1: 2, 3: 4}

		var value map[*int]*int
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		for k, v := range value {
			switch *k {
			case 1:
				if *v != 2 {
					t.Fatal("expected 2, got", *v)
				}
			case 3:
				if *v != 4 {
					t.Fatal("expected 4, got", *v)
				}
			default:
				t.Fatal("unexpected key", *k)
			}
		}
	})

	t.Run("int64 pointer keys and values", func(t *testing.T) {
		data := "\xA2\x01\x02\x03\x04" // {1: 2, 3: 4}

		var value map[int64]int64
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		if value[1] != 2 {
			t.Fatal("expected 2, got", value[1])
		}
		if value[3] != 4 {
			t.Fatal("expected 4, got", value[3])
		}
	})

	t.Run("float64 keys and values", func(t *testing.T) {
		data := "\xA2\xFB\x40\x09\x1E\xB8\x51\xEB\x85\x1F\x01\xFB\x40\x44\xA6\x66\x66\x66\x66\x66\x02" // {3.14:1, 41.3: 2}

		var value map[float64]float64
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		if value[41.3] != 2 {
			t.Fatal("expected 2, got", value[1])
		}
		if value[3.14] != 1 {
			t.Fatal("expected 4, got", value[3])
		}
	})

	t.Run("float64 pointer keys and values", func(t *testing.T) {
		data := "\xA2\xFB\x40\x09\x1E\xB8\x51\xEB\x85\x1F\x01\xFB\x40\x44\xA6\x66\x66\x66\x66\x66\x02" // {3.14:1, 41.3: 2}

		var value map[*float64]*float64
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		for k, v := range value {
			switch *k {
			case 3.14:
				if *v != 1 {
					t.Fatal("expected 1, got", *v)
				}
			case 41.3:
				if *v != 2 {
					t.Fatal("expected 2, got", *v)
				}
			default:
				t.Fatal("unexpected key", *k)
			}
		}
	})

	t.Run("float64 keys and int values", func(t *testing.T) {
		data := "\xA2\xFB\x40\x09\x1E\xB8\x51\xEB\x85\x1F\x01\xFB\x40\x44\xA6\x66\x66\x66\x66\x66\x02" // {3.14:1, 41.3: 2}

		var value map[float64]int
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		if value[3.14] != 1 {
			t.Fatal("expected 2, got", value[1])
		}
		if value[41.3] != 2 {
			t.Fatal("expected 4, got", value[3])
		}
	})

	t.Run("uint8 keys and values", func(t *testing.T) {
		data := "\xA2\x01\x02\x03\x04" // {1:2, 3: 4}

		var value map[uint8]uint8
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		if value[1] != 2 {
			t.Fatal("expected 2, got", value[1])
		}
		if value[3] != 4 {
			t.Fatal("expected 4, got", value[3])
		}
	})

	t.Run("uint8 pointer keys and values", func(t *testing.T) {
		data := "\xA2\x01\x02\x03\x04" // {1:2, 3: 4}

		var value map[*uint8]*uint8
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
		if err != nil {
			t.Fatal(err)
		}

		if len(value) != 2 {
			t.Fatal("expected 2, got", len(value))
		}
		for k, v := range value {
			switch *k {
			case 1:
				if *v != 2 {
					t.Fatal("expected 2, got", *v)
				}
			case 3:
				if *v != 4 {
					t.Fatal("expected 4, got", *v)
				}
			default:
				t.Fatal("unexpected key", *k)
			}
		}
	})
}

var benchDecodeMapValue map[uint8]uint8

// BenchmarkDecodeMap benchmarks decoding a map.
func BenchmarkDecodeMap(b *testing.B) {
	data := "\xA2\x01\x02\x03\x04" // {1:2, 3: 4}

	for i := 0; i < b.N; i++ {
		err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&benchDecodeMapValue)
		if err != nil {
			b.Fatal(err)
		}
	}

	// Double check that the value is correct.
	if len(benchDecodeMapValue) != 2 {
		b.Fatal("expected 2, got", len(benchDecodeMapValue))
	}

	if benchDecodeMapValue[1] != 2 {
		b.Fatal("expected 2, got", benchDecodeMapValue[1])
	}

	if benchDecodeMapValue[3] != 4 {
		b.Fatal("expected 4, got", benchDecodeMapValue[3])
	}
}

var benchDecodeMalformedValue []uint8

func BenchmarkDecodeMalformed(b *testing.B) {
	b.StopTimer()

	// This is a malformed CBOR data stream.
	data := []byte{0x9B, 0x00, 0x00, 0x42, 0xFA, 0x42, 0xFA, 0x42, 0xFA, 0x42} // designed to cause an error (large array)

	var err error

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		err = cbor.NewDecoder(bytes.NewBuffer(data)).Decode(&benchDecodeMalformedValue)
		if err == nil {
			b.Fatal("expected error")
		}
	}
}
