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

func RecursiveDownloadWorker(c *fasthttp.Client, baseUrl, baseDir string, jt *jobtracker.JobTracker) {
	for {
		select {
		case f, ok := <-jt.Queue:
			if ok {
				recursiveDownload(c, baseUrl, baseDir, f, jt)
			}
		default:
			if !jt.HasWork() {
				return
			}
			jt.Nap()
		}
	}
}

func recursiveDownload(c *fasthttp.Client, baseUrl, baseDir, f string, jt *jobtracker.JobTracker) {
	jt.StartWork()
	defer jt.EndWork()

	checkRatelimted()

	filePath := utils.Url(baseDir, f)
	isDir := strings.HasSuffix(f, "/")
	if !isDir && utils.Exists(filePath) {
		log.Info().Str("file", filePath).Msg("already fetched, skipping redownload")
		return
	}
	uri := utils.Url(baseUrl, f)
	code, body, err := c.Get(nil, uri)
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
