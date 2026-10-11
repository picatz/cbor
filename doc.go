// Package cbor implements a minimal Concise Binary Object Representation (CBOR)
// encoder and decoder in a similar style to the encoding/json package.
//
// It implements a bounded definite-length subset of RFC 8949. Indefinite
// lengths, composite map keys, and semantic tag interpretation are unsupported.
//
// https://www.rfc-editor.org/rfc/rfc8949.html
package cbor
