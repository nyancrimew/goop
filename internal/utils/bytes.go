package utils

import (
	"unicode"
	"unicode/utf8"
)

var asciiSpace = [256]uint8{'\t': 1, '\n': 1, '\v': 1, '\f': 1, '\r': 1, ' ': 1}

func IsEmptyBytes(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	for i := 0 ; i < len(b); i++ {
		c := b[i]
		if c == 0 {
			continue
		}
		if utf8.RuneStart(c) {
			r, _ := utf8.DecodeRune(b[i:])
			if !unicode.IsSpace(r) {
				return false
			}
		}
		if asciiSpace[c] == 0 {
			return false
		}
	}
	return true
}
