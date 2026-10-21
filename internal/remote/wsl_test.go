package remote

import (
	"testing"
	"unicode/utf16"
)

func TestWSL(t *testing.T) {
	if d, ok := WSL(WSLHost("Ubuntu-24.04")); !ok || d != "Ubuntu-24.04" {
		t.Errorf("%q %v", d, ok)
	}
	for _, host := range []string{"box", "ada@wsl", "wsl+", "wsl"} {
		if _, ok := WSL(host); ok {
			t.Errorf("%q read as WSL's", host)
		}
	}
}

func TestDecodeWSL(t *testing.T) {
	const list = "Ubuntu\r\nDebian\r\n"
	var wide []byte
	for _, u := range utf16.Encode([]rune(list)) {
		wide = append(wide, byte(u), byte(u>>8))
	}
	for _, in := range [][]byte{[]byte(list), wide, append([]byte{0xff, 0xfe}, wide...)} {
		if got := decodeWSL(in); got != list {
			t.Errorf("%q: %q", in, got)
		}
	}
}
