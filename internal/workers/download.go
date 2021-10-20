package workers

import (
	"io/ioutil"
	"os"

	"github.com/deletescape/goop/internal/jobtracker"
	"github.com/deletescape/goop/internal/utils"
	"github.com/phuslu/log"
	"github.com/valyala/fasthttp"
)

func DownloadWorker(c *fasthttp.Client, baseUrl, baseDir string, jt *jobtracker.JobTracker, allowHtml, allowEmpty bool) {
	for {
		select {
		case file := <-jt.Queue:
			downloadWork(c, baseUrl, baseDir, file, jt, allowHtml, allowEmpty)
		default:
			if !jt.HasWork() {
				return
			}
			jt.Nap()
		}
	}
}

func downloadWork(c *fasthttp.Client, baseUrl, baseDir, file string, jt *jobtracker.JobTracker, allowHtml, allowEmpty bool) {
	jt.StartWork()
	defer jt.EndWork()

	if file == "" {
		return
	}
	checkRatelimted()

	targetFile := utils.Url(baseDir, file)
	if utils.Exists(targetFile) {
		log.Info().Str("file", targetFile).Msg("already fetched, skipping redownload")
		return
	}
	uri := utils.Url(baseUrl, file)
	code, body, err := c.Get(nil, uri)
	if err == nil && code != 200 {
		if code == 429 {
			setRatelimited()
			jt.AddJob(file)
			return
		}
		log.Warn().Str("uri", uri).Int("code", code).Msg("couldn't fetch file")
		return
	} else if err != nil {
		log.Error().Str("uri", uri).Int("code", code).Err(err).Msg("couldn't fetch file")
		return
	}

	if !allowHtml && utils.IsHtml(body) {
		log.Warn().Str("uri", uri).Msg("file appears to be html, skipping")
		return
	}
	if !allowEmpty && utils.IsEmptyBytes(body) {
		log.Warn().Str("uri", uri).Msg("file appears to be empty, skipping")
		return
	}
	if err := utils.CreateParentFolders(targetFile); err != nil {
		log.Error().Str("uri", uri).Str("file", targetFile).Err(err).Msg("couldn't create parent directories")
		return
	}
	if err := ioutil.WriteFile(targetFile, body, os.ModePerm); err != nil {
		log.Error().Str("uri", uri).Str("file", targetFile).Err(err).Msg("clouldn't write file")
		return
	}
	log.Info().Str("uri", uri).Str("file", file).Msg("fetched file")
}
