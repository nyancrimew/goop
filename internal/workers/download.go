package workers

import (
	"io/ioutil"
	"os"

	"github.com/deletescape/goop/internal/utils"
	"github.com/deletescape/jobtracker"
	"github.com/phuslu/log"
	"github.com/valyala/fasthttp"
)

type DownloadContext struct {
	C           *fasthttp.Client
	BaseUrl     string
	BaseDir     string
	AllowHtml   bool
	AlllowEmpty bool
}

func DownloadWorker(jt *jobtracker.JobTracker, file string, context jobtracker.Context) {
	c := context.(DownloadContext)
	checkRatelimted()

	targetFile := utils.Url(c.BaseDir, file)
	if utils.Exists(targetFile) {
		log.Info().Str("file", targetFile).Msg("already fetched, skipping redownload")
		return
	}
	uri := utils.Url(c.BaseUrl, file)
	code, body, err := c.C.Get(nil, uri)
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

	if !c.AllowHtml && utils.IsHtml(body) {
		log.Warn().Str("uri", uri).Msg("file appears to be html, skipping")
		return
	}
	if !c.AlllowEmpty && utils.IsEmptyBytes(body) {
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
