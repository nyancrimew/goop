package workers

import (
	"fmt"
	"github.com/deletescape/goop/internal/utils"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"os"
	"sync"
)

func DownloadWorker(c *fasthttp.Client, queue <-chan string, baseUrl, baseDir string, wg *sync.WaitGroup) {
	defer wg.Done()
	for file := range queue {
		if file == "" {
			continue
		}
		targetFile := utils.Url(baseDir, file)
		if utils.Exists(targetFile) {
			fmt.Printf("%s was downloaded already, skipping\n", targetFile)
			continue
		}
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
			}
		}
	}
}
