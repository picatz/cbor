package cbor_test

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/picatz/cbor"
	// otherCbor "github.com/fxamacker/cbor/v2"
)

func ExampleDecoder() {
	const data = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64" // {"hello": "world"}

	var value map[string]string
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		panic(err)
	}

	// Output: world
	fmt.Println(value["hello"])
}

func TestDecoderStructTag(t *testing.T) {
	const data = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64" // {"hello": "world"}

	type example struct {
		Hello string `cbor:"hello"`
	}

	var value example
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		panic(err)
	}

	// Output: world
	fmt.Println(value.Hello)
}

type testStructHello struct {
	Hello string `cbor:"hello"`
}

func TestDecoderStruct_tag(t *testing.T) {
	const cborStream = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64" // {"hello": "world"}

	var value testStructHello
	err := cbor.NewDecoder(bytes.NewBufferString(cborStream)).Decode(&value)
	if err != nil {
		panic(err)
	}

	if value.Hello != "world" {
		t.Fatal("expected world, got", value.Hello)
	}
}

func TestDecoderMap(t *testing.T) {
	const cborStream = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64" // {"hello": "world"}

	var value map[string]string
	err := cbor.NewDecoder(bytes.NewBufferString(cborStream)).Decode(&value)
	if err != nil {
		panic(err)
	}

	if value["hello"] != "world" {
		t.Fatal("expected world, got", value["hello"])
	}
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

func TestDecodeUInt(t *testing.T) {
	data := "\x01"

	var value uint
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != 1 {
		t.Fatal("expected 1, got", value)
	}
}

func TestDecodeUIntPointer(t *testing.T) {
	data := "\x01"

	var value *uint
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != 1 {
		t.Fatal("expected 1, got", value)
	}
}

func TestDecodeUInt32(t *testing.T) {
	data := "\x01"

	var value uint32
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if value != 1 {
		t.Fatal("expected 1, got", value)
	}
}

func TestDecodeUInt32Pointer(t *testing.T) {
	data := "\x01"

	var value *uint32
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		t.Fatal(err)
	}

	if *value != 1 {
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
				if err != nil && !strings.Contains(err.Error(), "EOF") {
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
				if err != nil && !strings.Contains(err.Error(), "EOF") {
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
//
// $ go test -benchmem -run=^$ -bench ^BenchmarkDecodeMap$ github.com/picatz/cbor -v
//
// goos: darwin
// goarch: arm64
// pkg: github.com/picatz/cbor
// BenchmarkDecodeMap
// BenchmarkDecodeMap-8   	 4109782	       290.6 ns/op	     120 B/op	      12 allocs/op
// PASS
// ok  	github.com/picatz/cbor	1.579s
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

// $ go test -benchmem -run=^$ -bench ^BenchmarkDecodeMalformed$ github.com/picatz/cbor -v
//
// goos: darwin
// goarch: arm64
// pkg: github.com/picatz/cbor
// BenchmarkDecodeMalformed
// BenchmarkDecodeMalformed-8   	 4152103	       289.8 ns/op	     712 B/op	       7 allocs/op
func BenchmarkDecodeMalformed(b *testing.B) {
	// This is a malformed CBOR data stream.
	data := []byte{0x9B, 0x00, 0x00, 0x42, 0xFA, 0x42, 0xFA, 0x42, 0xFA, 0x42} // designed to cause an error (large array)

	for i := 0; i < b.N; i++ {
		err := cbor.NewDecoder(bytes.NewBuffer(data)).Decode(&benchDecodeMalformedValue)
		if err == nil {
			b.Fatal("expected error")
		}
	}
}

type claims struct {
	Iss string `cbor:"1,keyasint"`
	Sub string `cbor:"2,keyasint"`
	Aud string `cbor:"3,keyasint"`
	Exp int    `cbor:"4,keyasint"`
	Nbf int    `cbor:"5,keyasint"`
	Iat int    `cbor:"6,keyasint"`
	Cti []byte `cbor:"7,keyasint"`
}

func TestDecodeCWTClaims(t *testing.T) {
	// Data from https://tools.ietf.org/html/rfc8392#appendix-A section A.1
	data, err := hex.DecodeString("a70175636f61703a2f2f61732e6578616d706c652e636f6d02656572696b77037818636f61703a2f2f6c696768742e6578616d706c652e636f6d041a5612aeb0051a5610d9f0061a5610d9f007420b71")
	if err != nil {
		t.Fatal("hex.DecodeString:", err)
	}
	var v claims
	if err := cbor.NewDecoder(bytes.NewReader(data)).Decode(&v); err != nil {
		t.Fatal(err)
	}

	if v.Iss != "coap://as.example.com" {
		t.Fatal("unexpected Iss:", v.Iss)
	}

	if v.Sub != "erikw" {
		t.Fatal("unexpected Sub:", v.Sub)
	}

	if v.Aud != "coap://light.example.com" {
		t.Fatal("unexpected Aud:", v.Aud)
	}

	if v.Exp != 1444064944 {
		t.Fatal("unexpected Exp:", v.Exp)
	}

	if v.Nbf != 1443944944 {
		t.Fatal("unexpected Nbf:", v.Nbf)
	}

	if v.Iat != 1443944944 {
		t.Fatal("unexpected Iat:", v.Iat)
	}

	if !bytes.Equal(v.Cti, []byte{0x0b, 0x71}) {
		t.Fatal("unexpected Cti:", v.Cti)
	}
}

// $ go test -benchmem -run=^$ -bench ^BenchmarkUnmarshalString$ github.com/picatz/cbor -v
//
// goos: darwin
// goarch: arm64
// pkg: github.com/picatz/cbor
// BenchmarkUnmarshalString
// BenchmarkUnmarshalString-8   	 7057992	       159.6 ns/op	     656 B/op	       5 allocs/op
func BenchmarkUnmarshalString(b *testing.B) {
	data, err := hex.DecodeString("6B68656C6C6F20776F726C64")
	if err != nil {
		b.Fatal("hex.DecodeString:", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v string
		if err := cbor.Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

// $ go test -benchmem -run=^$ -bench ^BenchmarkUnmarshalCWTClaims$ github.com/picatz/cbor -v
//
// goos: darwin
// goarch: arm64
// pkg: github.com/picatz/cbor
// BenchmarkUnmarshalCWTClaims
// BenchmarkUnmarshalCWTClaims-8   	 1915150	       541.9 ns/op	     773 B/op	       7 allocs/op
func BenchmarkUnmarshalCWTClaims(b *testing.B) {
	// Data from https://tools.ietf.org/html/rfc8392#appendix-A section A.1
	//
	// {1: "coap://as.example.com", 2: "erikw", 3: "coap://light.example.com", 4: 1444064944, 5: 1443944944, 6: 1443944944, 7: h'0B71'}
	data, err := hex.DecodeString("a70175636f61703a2f2f61732e6578616d706c652e636f6d02656572696b77037818636f61703a2f2f6c696768742e6578616d706c652e636f6d041a5612aeb0051a5610d9f0061a5610d9f007420b71")
	if err != nil {
		b.Fatal("hex.DecodeString:", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v claims
		if err := cbor.Unmarshal(data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

// Competitive analysis.
//
// $ go test -benchmem -run=^$ -bench ^BenchmarkUnmarshalCWTClaims_other$ github.com/picatz/cbor -v
//
// goos: darwin
// goarch: arm64
// pkg: github.com/picatz/cbor
// BenchmarkUnmarshalCWTClaims_other
// BenchmarkUnmarshalCWTClaims_other-8   	 3439036	       351.2 ns/op	     160 B/op	       6 allocs/op
// PASS
// ok  	github.com/picatz/cbor	1.735s
//
// // https://github.com/fxamacker/cbor/blob/25ddb46501d04685db150a11d06167816cb85c12/bench_test.go#L500
//
// func BenchmarkUnmarshalCWTClaims_other(b *testing.B) {
// 	// Data from https://tools.ietf.org/html/rfc8392#appendix-A section A.1
// 	b.StopTimer()
// 	// Data from https://tools.ietf.org/html/rfc8392#appendix-A section A.1
// 	data, err := hex.DecodeString("a70175636f61703a2f2f61732e6578616d706c652e636f6d02656572696b77037818636f61703a2f2f6c696768742e6578616d706c652e636f6d041a5612aeb0051a5610d9f0061a5610d9f007420b71")
// 	if err != nil {
// 		b.Fatal("hex.DecodeString:", err)
// 	}
//
// 	b.StartTimer()
// 	for i := 0; i < b.N; i++ {
// 		var v claims
// 		if err := otherCbor.Unmarshal(data, &v); err != nil {
// 			b.Fatal("Unmarshal:", err)
// 		}
// 	}
// }
