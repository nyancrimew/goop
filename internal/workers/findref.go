package workers

import (
	"io/ioutil"
	"os"
	"regexp"
	"sync"

	"github.com/deletescape/goop/internal/jobtracker"
	"github.com/deletescape/goop/internal/utils"
	"github.com/phuslu/log"
	"github.com/valyala/fasthttp"
)

var refRegex = regexp.MustCompile(`(?m)(refs(/[a-zA-Z0-9\-\.\_\*]+)+)`)
var branchRegex = regexp.MustCompile(`(?m)branch ["'](.+)["']`)

var checkedRefs = make(map[string]bool)
var checkedRefsMutex sync.Mutex

func FindRefWorker(c *fasthttp.Client, baseUrl, baseDir string, jt *jobtracker.JobTracker) {
	for {
		select {
		case path := <-jt.Queue:
			findRefWork(c, baseUrl, baseDir, path, jt)
		default:
			if !jt.HasWork() {
				return
			}
			jobtracker.Nap()
		}
	}
}

func findRefWork(c *fasthttp.Client, baseUrl, baseDir, path string, jt *jobtracker.JobTracker) {
	jt.StartWork()
	defer jt.EndWork()

	// TODO: do we still need this check here?
	if path == "" {
		return
	}

	checkedRefsMutex.Lock()
	if checked, ok := checkedRefs[path]; checked && ok {
		// Ref has already been checked
		checkedRefsMutex.Unlock()
		return
	} else {
		checkedRefs[path] = true
	}

	targetFile := utils.Url(baseDir, path)
	if utils.Exists(targetFile) {
		log.Info().Str("file", targetFile).Msg("already fetched, skipping redownload")
		content, err := ioutil.ReadFile(targetFile)
		if err != nil {
			log.Error().Str("file", targetFile).Err(err).Msg("error while reading file")
		}
		for _, ref := range refRegex.FindAll(content, -1) {
			jt.AddJob(utils.Url(".git", string(ref)))
			jt.AddJob(utils.Url(".git/logs", string(ref)))
		}
		if path == ".git/config" || path == ".git/FETCH_HEAD" {
			// TODO check the actual origin instead of just assuming origin here
			for _, branch := range branchRegex.FindAllSubmatch(content, -1) {
				jt.AddJob(utils.Url(".git/refs/remotes/origin", string(branch[1])))
				jt.AddJob(utils.Url(".git/logs/refs/remotes/origin", string(branch[1])))
			}
		}
		return
	}

	uri := utils.Url(baseUrl, path)
	code, body, err := c.Get(nil, uri)
	if err == nil && code != 200 {
		if code == 429 {
			setRatelimited()
			jt.AddJob(path)
			return
		}
		log.Warn().Str("uri", uri).Int("code", code).Msg("failed to fetch ref")
		return
	} else if err != nil {
		log.Error().Str("uri", uri).Int("code", code).Err(err).Msg("failed to fetch ref")
		return
	}

	if utils.IsHtml(body) {
		log.Warn().Str("uri", uri).Msg("file appears to be html, skipping")
		return
	}
	if utils.IsEmptyBytes(body) {
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

	log.Info().Str("uri", uri).Msg("fetched ref")

	for _, ref := range refRegex.FindAll(body, -1) {
		jt.AddJob(utils.Url(".git", string(ref)))
		jt.AddJob(utils.Url(".git/logs", string(ref)))
	}
	if path == ".git/config" || path == ".git/FETCH_HEAD" {
		// TODO check the actual origin instead of just assuming origin here
		for _, branch := range branchRegex.FindAllSubmatch(body, -1) {
			jt.AddJob(utils.Url(".git/refs/remotes/origin", string(branch[1])))
			jt.AddJob(utils.Url(".git/logs/refs/remotes/origin", string(branch[1])))
		}
	}
}
