package workers

import (
	"fmt"
	"github.com/deletescape/goop/internal/utils"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
	"sync"
	"time"
)

// TODO: more dedupe stuff
func FindObjectsWorker(c *fasthttp.Client, jobs chan string, baseUrl, baseDir string, wg *sync.WaitGroup, storage *filesystem.ObjectStorage) {
	wg.Add(1)
	defer wg.Done()
	var ctr int
	for {
		select {
		case obj := <-jobs:
			file := fmt.Sprintf(".git/objects/%s/%s", obj[:2], obj[2:])
			uri := utils.Url(baseUrl, file)
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
				fullPath := utils.Url(baseDir, file)
				if err := utils.CreateParentFolders(fullPath); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				if err := ioutil.WriteFile(fullPath, body, os.ModePerm); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}

				encObj, err := storage.EncodedObject(plumbing.AnyObject, plumbing.NewHash(obj))
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				decObj, err := object.DecodeObject(storage, encObj)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					continue
				}
				referencedHashes := utils.GetReferencedHashes(decObj)
				for _, h := range referencedHashes {
					jobs <- h
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
