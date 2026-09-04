package utils

import "slices"

var ignoredErrors = []string{
	"too many redirects detected when doing the request",
}

func IgnoreError(err error) bool {
	return slices.Contains(ignoredErrors, err.Error())
}
