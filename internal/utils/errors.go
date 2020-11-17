package utils

var ignoredErrors = []string{
	"too many redirects detected when doing the request",
}

func IgnoreError(err error) bool {
	return StringsContain(ignoredErrors, err.Error())
}
