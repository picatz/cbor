// Package interop holds differential tests between github.com/picatz/cbor and
// an independent implementation, github.com/fxamacker/cbor/v2. It is a
// separate module so the main module keeps no third-party dependencies.
//
//	cd interop && go test ./... && go test -run '^$' -fuzz FuzzDifferential -fuzztime 30s
package interop
