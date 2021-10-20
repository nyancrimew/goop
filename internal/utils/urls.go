package utils

import "strings"

//TODO: replace all uses of this with the proper path utils
func Url(base, path string) string {
	return strings.TrimSuffix(base, "/") + "/" + strings.TrimPrefix(path, "/")
}
