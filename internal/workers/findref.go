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

func FindRefWorker(c *fasthttp.Client, jobs chan string, baseUrl, baseDir string, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	var ctr int
	for {
		select {
		case path := <-jobs:
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
				if err := utils.CreateParentFolders(utils.Url(baseDir, path)); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				if err := ioutil.WriteFile(utils.Url(baseDir, path), body, os.ModePerm); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}

				for _, ref := range refRegex.FindAll(body, -1) {
					jobs <- utils.Url(".git", string(ref))
				}
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
