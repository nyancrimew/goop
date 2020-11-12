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
			filePath := utils.Url(baseDir, f)
			isDir := strings.HasSuffix(f, "/")
			if !isDir && utils.Exists(filePath) {
				fmt.Printf("%s was downloaded already, skipping\n", filePath)
				continue
			}
			uri := utils.Url(baseUrl, f)
			code, body, err := c.Get(nil, uri)
			fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				continue
			}
			if isDir {
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
				if err := utils.CreateParentFolders(filePath); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				if err := ioutil.WriteFile(filePath, body, os.ModePerm); err != nil {
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
