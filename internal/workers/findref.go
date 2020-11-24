package workers

import (
	"fmt"
	"github.com/deletescape/goop/internal/utils"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
	"regexp"
	"sync"
	"time"
)

var refRegex = regexp.MustCompile(`(?m)(refs(/[a-zA-Z0-9\-\.\_\*]+)+)`)
var branchRegex = regexp.MustCompile(`(?m)branch ["'](.+)["']`)

var checkedRefs = make(map[string]bool)
var checkedRefsMutex sync.Mutex

func FindRefWorker(c *fasthttp.Client, queue chan string, baseUrl, baseDir string, wg *sync.WaitGroup) {
	defer wg.Done()
	var ctr int
	for {
		select {
		case path := <-queue:
			checkRatelimted()
			if path == "" {
				continue
			}
			ctr = 0
			checkedRefsMutex.Lock()
			if checked, ok := checkedRefs[path]; checked && ok {
				// Ref has already been checked
				checkedRefsMutex.Unlock()
				continue
			} else {
				checkedRefs[path] = true
			}
			checkedRefsMutex.Unlock()
			targetFile := utils.Url(baseDir, path)
			if utils.Exists(targetFile) {
				fmt.Printf("%s was downloaded already, skipping\n", targetFile)
				content, err := ioutil.ReadFile(targetFile)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
				}
				for _, ref := range refRegex.FindAll(content, -1) {
					queue <- utils.Url(".git", string(ref))
					queue <- utils.Url(".git/logs", string(ref))
				}
				if path == ".git/config" || path == ".git/FETCH_HEAD" {
					// TODO check the actual origin instead of just assuming origin here
					for _, branch := range branchRegex.FindAllSubmatch(content, -1) {
						queue <- utils.Url(".git/refs/remotes/origin", string(branch[1]))
						queue <- utils.Url(".git/logs/refs/remotes/origin", string(branch[1]))
					}
				}
				continue
			}
			uri := utils.Url(baseUrl, path)
			code, body, err := c.Get(nil, uri)
			fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				continue
			}
			if code == 200 {
				if utils.IsHtml(body) {
					fmt.Printf("warning: %s appears to be an html file, skipping\n", uri)
					continue
				}
				if utils.IsEmptyBytes(body) {
					fmt.Printf("warning: %s appears to be an empty file, skipping\n", uri)
					continue
				}
				if err := utils.CreateParentFolders(targetFile); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				if err := ioutil.WriteFile(targetFile, body, os.ModePerm); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}

				for _, ref := range refRegex.FindAll(body, -1) {
					queue <- utils.Url(".git", string(ref))
					queue <- utils.Url(".git/logs", string(ref))
				}
				if path == ".git/config" || path == ".git/FETCH_HEAD" {
					// TODO check the actual origin instead of just assuming origin here
					for _, branch := range branchRegex.FindAllSubmatch(body, -1) {
						queue <- utils.Url(".git/refs/remotes/origin", string(branch[1]))
					}
				}
			} else if code == 429 {
				setRatelimited()
				queue <- path
			}
		default:
			// TODO: get rid of dirty hack somehow
			if ctr >= graceTimes {
				return
			}
			ctr++
			time.Sleep(gracePeriod)
		}
	}
}
