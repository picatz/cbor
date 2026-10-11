package cbor_test

import (
	"github.com/picatz/cbor"
	"testing"
)

func TestFIDOReview(t *testing.T) {
	for _, name := range []string{"boolean-option", "truncated-text", "duplicate-key", "trailing-data", "integer-overflow"} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if failure := recover(); failure != nil {
					t.Errorf("decoder panicked: %v", failure)
				}
			}()
			switch name {
			case "boolean-option":
				var value map[string]bool
				if err := cbor.Unmarshal([]byte{0xa1, 0x62, 'r', 'k', 0xf5}, &value); err != nil || !value["rk"] {
					t.Fatalf("boolean map: %v, %v", value, err)
				}
			case "truncated-text":
				var value string
				if err := cbor.Unmarshal([]byte{0x63, 'a'}, &value); err == nil {
					t.Fatalf("accepted truncated text: %q", value)
				}
			case "duplicate-key":
				var value map[int]int
				if err := cbor.Unmarshal([]byte{0xa2, 1, 2, 1, 3}, &value); err == nil {
					t.Fatalf("accepted duplicate key: %v", value)
				}
			case "trailing-data":
				var value uint64
				if err := cbor.Unmarshal([]byte{1, 2}, &value); err == nil {
					t.Fatal("accepted trailing item")
				}
			case "integer-overflow":
				var value int64
				if err := cbor.Unmarshal([]byte{0x1b, 255, 255, 255, 255, 255, 255, 255, 255}, &value); err == nil {
					t.Fatalf("accepted overflow: %d", value)
				}
			}
		})
	}
}
