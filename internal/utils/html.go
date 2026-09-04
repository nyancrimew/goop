package utils

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const htmlTag = "<html"

func IsHtml(body []byte) bool {
	return bytes.Contains(body, []byte(htmlTag))
}

func GetIndexedFiles(body []byte, basePath string) ([]string, error) {
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	var files []string
	var exitErr error
	doc.Find("a").EachWithBreak(func(_ int, link *goquery.Selection) bool {
		lnk, err := url.Parse(link.AttrOr("href", ""))
		if err != nil {
			exitErr = err
			return false
		}
		path := strings.TrimPrefix(lnk.Path, basePath)
		if path != "" &&
			path != "." &&
			path != "./" &&
			!strings.HasPrefix(path, "..") &&
			!strings.HasPrefix(path, "/") &&
			lnk.Scheme == "" &&
			lnk.Host == "" {
			files = append(files, path)
		}
		return true
	})
	return files, exitErr
}
