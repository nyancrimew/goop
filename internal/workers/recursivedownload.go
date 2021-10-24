package workers

import (
	"io/ioutil"
	"net/url"
	"os"
	"strings"

	"github.com/deletescape/goop/internal/jobtracker"
	"github.com/deletescape/goop/internal/utils"
	"github.com/phuslu/log"
	"github.com/valyala/fasthttp"
)

type RecursiveDownloadContext struct {
	C       *fasthttp.Client
	BaseUrl string
	BaseDir string
}

func RecursiveDownloadWorker(jt *jobtracker.JobTracker, f string, context jobtracker.Context) {
	c := context.(RecursiveDownloadContext)

	checkRatelimted()

	filePath := utils.Url(c.BaseDir, f)
	isDir := strings.HasSuffix(f, "/")
	if !isDir && utils.Exists(filePath) {
		log.Info().Str("file", filePath).Msg("already fetched, skipping redownload")
		return
	}
	uri := utils.Url(c.BaseUrl, f)
	code, body, err := c.C.Get(nil, uri)
	if err == nil && code != 200 {
		if code == 429 {
			setRatelimited()
			jt.AddJob(f)
			return
		}
		log.Warn().Str("uri", uri).Int("code", code).Msg("failed to fetch file")
		return
	} else if err != nil {
		log.Error().Str("uri", uri).Int("code", code).Err(err).Msg("failed to fetch file")
		return
	}

	if isDir {
		if !utils.IsHtml(body) {
			log.Warn().Str("uri", uri).Msg("not a directory index, skipping")
			return
		}

		lnk, _ := url.Parse(uri)
		indexedFiles, err := utils.GetIndexedFiles(body, lnk.Path)
		if err != nil {
			log.Error().Str("uri", uri).Err(err).Msg("couldn't get list of indexed files")
			return
		}
		log.Info().Str("uri", uri).Msg("fetched directory listing")
		for _, idxf := range indexedFiles {
			jt.AddJob(utils.Url(f, idxf))
		}
	} else {
		if err := utils.CreateParentFolders(filePath); err != nil {
			log.Error().Str("file", filePath).Err(err).Msg("couldn't create parent directories")
			return
		}
		if err := ioutil.WriteFile(filePath, body, os.ModePerm); err != nil {
			log.Error().Str("file", filePath).Err(err).Msg("couldn't write to file")
			return
		}
		log.Info().Str("uri", uri).Msg("fetched file")
	}
}
