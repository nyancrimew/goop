package utils

import (
	"bytes"
	"net/url"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

var htmlTag = []byte{'<', 'h', 't', 'm', 'l'}

func IsHtml(body []byte) bool {
	return bytes.Contains(body, htmlTag)
}

func GetIndexedFiles(body []byte) ([]string, error) {
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
		if lnk.Path != "" &&
			lnk.Path != "." &&
			lnk.Path != ".." &&
			!strings.HasPrefix(lnk.Path, "/") &&
			lnk.Scheme == "" &&
			lnk.Host == "" {
			files = append(files, lnk.Path)
		}
		return true
	})
	return files, exitErr
}
