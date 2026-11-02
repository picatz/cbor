package cbor

import (
	"testing"
	"unicode/utf8"
)

// FuzzValidText checks that validText agrees with utf8.ValidString.
func FuzzValidText(f *testing.F) {
	for _, seed := range []string{"", "a", "login.example.com", "Alice Johnson — Platform", "\xff", "1234567\xff", "12345678\xc3", "\xed\xa0\x80", "\xf4\x90\x80\x80", "12345678€", "€12345678"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if got, want := validText(text), utf8.ValidString(text); got != want {
			t.Fatalf("validText(%q) = %v, utf8.ValidString = %v", text, got, want)
		}
	})
}
