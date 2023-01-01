# CBOR
 
[Concise Binary Object Representation](https://www.rfc-editor.org/rfc/rfc8949.html) (CBOR) is a schemaless data format 
designed to be small and extensible. This package implements a minimal Concise Binary Object Representation (CBOR)
encoder and decoder in a similar style to the [`encoding/json`](https://pkg.go.dev/encoding/json) package.

```go
package main

import (
	"bytes"
	"fmt"

	"github.com/picatz/cbor"
)

func main() {
	const cborStream = "\xA1\x65\x68\x65\x6C\x6C\x6F\x65\x77\x6F\x72\x6C\x64" // {"hello": "world"}

	var value map[string]string
	err := cbor.NewDecoder(bytes.NewBufferString(cborStream)).Decode(&value)
	if err != nil {
		panic(err)
	}

	// Output: world
	fmt.Println(value["hello"])
}
```
