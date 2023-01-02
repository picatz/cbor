# CBOR
 
[Concise Binary Object Representation](https://www.rfc-editor.org/rfc/rfc8949.html) (CBOR) is a schemaless data format 
designed to be small and extensible. This package implements a minimal CBOR encoder and decoder in a similar style to 
the [`encoding/json`](https://pkg.go.dev/encoding/json) package.

```go
package main

import (
	"bytes"
	"fmt"

	"github.com/picatz/cbor"
)

// This code shows how to encode and decode the CBOR data format.
func main() {
	// CBOR encoding of the value {"hello": "world"}.
	const data = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64"

	// Create a new cbor.Decoder using bytes.NewBufferString(data) as its source,
	// and then Decode the CBOR data into the value map[string]string.
	var value map[string]string
	err := cbor.NewDecoder(bytes.NewBufferString(data)).Decode(&value)
	if err != nil {
		panic(err)
	}

	// Output: world
	fmt.Println(value["hello"])

	// Encode the value map[string]string using the cbor.NewEncoder.
	//
	// Note: this currently doesn't encode the data exactly the same 
	//       as it got it (non-canonical).
	var buf = bytes.NewBuffer(nil)
	err = cbor.NewEncoder(buf).Encode(value)
	if err != nil {
		panic(err)
	}

	// Output: b80278036f6e6501780374776f02
	fmt.Printf("%x\n", buf.Bytes())
}
```
