package workers

import (
	"fmt"
	"github.com/deletescape/goop/internal/utils"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
	"strings"
	"sync"
	"time"
)

func RecursiveDownloadWorker(c *fasthttp.Client, queue chan string, baseUrl, baseDir string, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	var ctr int
	for {
		select {
		case f := <-queue:
			if f == "" {
				continue
			}
			ctr = 0
			uri := utils.Url(baseUrl, f)
			code, body, err := c.Get(nil, uri)
			fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				continue
			}
			if strings.HasSuffix(f, "/") {
				if !utils.IsHtml(body) {
					fmt.Printf("warning: %s doesn't appear to be an index", uri)
					continue
				}
				indexedFiles, err := utils.GetIndexedFiles(body)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				for _, idxf := range indexedFiles {
					queue <- utils.Url(f, idxf)
				}
			} else {
				if utils.IsHtml(body) {
					fmt.Printf("warning: %s doesn't appear to be a git file", uri)
					continue
				}
				if err := utils.CreateParentFolders(utils.Url(baseDir, f)); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				if err := ioutil.WriteFile(utils.Url(baseDir, f), body, os.ModePerm); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
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
