package workers

import (
	"fmt"
	"github.com/deletescape/goop/internal/utils"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
	"sync"
)

func DownloadWorker(c *fasthttp.Client, jobs <-chan string, baseUrl, baseDir string, wg *sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	for file := range jobs {
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
			if err := utils.CreateParentFolders(utils.Url(baseDir, file)); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				continue
			}
			if err := ioutil.WriteFile(utils.Url(baseDir, file), body, os.ModePerm); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
			}
		}
	}
}
