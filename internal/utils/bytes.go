package utils

import "bytes"

const emptyBytes = "\x00\t\n\v\f\r "

func IsEmptyBytes(b []byte) bool {
	return len(bytes.TrimLeft(b, emptyBytes)) == 0
}
