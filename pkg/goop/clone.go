package goop

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/deletescape/goop/internal/utils"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
)

var refPrefix = []byte{'r', 'e', 'f', ':'}

var c = &fasthttp.Client{
	TLSConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
}

func Clone(u, dir string) error {
	baseUrl := strings.TrimSuffix(u, "/")
	baseUrl = strings.TrimSuffix(baseUrl, "/HEAD")
	baseUrl = strings.TrimSuffix(baseUrl, "/.git")
	parsed, err := url.Parse(baseUrl)
	if err != nil {
		return err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	baseUrl = parsed.String()
	baseDir := dir
	if baseDir == "" {
		baseDir = parsed.Host
	}

	if !utils.Exists(baseDir) {
		err := os.MkdirAll(baseDir, os.ModePerm)
		if err != nil {
			return err
		}
	}
	if !utils.IsFolder(baseDir) {
		return fmt.Errorf("%s is not a directory", dir)
	}
	isEmpty, err := utils.IsEmpty(baseDir)
	if err != nil {
		return err
	}
	if !isEmpty {
		// todo: add option to force override of the folder here
		return fmt.Errorf("%s is not empty", baseDir)
	}
	return FetchGit(baseUrl, baseDir)
}

func DownloadRecursively(u, dir string, files []string, wg *sync.WaitGroup) error {
	os.MkdirAll(dir, os.ModePerm)
	for _, f := range files {
		uri := utils.Url(u, f)
		code, body, err := c.Get(nil, uri)
		fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
		if err != nil {
			return err
		}
		if strings.HasSuffix(f, "/") {
			wg.Add(1)
			go func(file, d string, b []byte) {
				defer wg.Done()
				if !utils.IsHtml(b) {
					fmt.Printf("warning: %s doesn't appear to be an index", uri)
					return
				}
				indexedFiles, err := utils.GetIndexedFiles(b)
				if err != nil {
					return
				}
				/* return */ DownloadRecursively(uri, utils.Url(d, file), indexedFiles, wg)
			}(f, dir, body)
		} else {
			wg.Add(1)
			go func(file, d string, b []byte) {
				defer wg.Done()
				if utils.IsHtml(b) {
					fmt.Printf("warning: %s doesn't appear to be a git file", uri)
					return
				}
				/* return */ ioutil.WriteFile(utils.Url(d, file), body, os.ModePerm)
			}(f, dir, body)
		}
	}
	return nil
}

func FetchGit(baseUrl, baseDir string) error {
	fmt.Printf("[-] Testing %s/.git/HEAD ", baseUrl)
	code, body, err := c.Get(nil, utils.Url(baseUrl, ".git/HEAD"))
	fmt.Printf("[%d]\n", code)
	if err != nil {
		return err
	}

	if code != 200 {
		return fmt.Errorf("error: %s/.git/HEAD does not exist", baseUrl)
	} else if !bytes.HasPrefix(body, refPrefix) {
		return fmt.Errorf("error: %s/.git/HEAD is not a git HEAD file", baseUrl)
	}

	fmt.Printf("[-] Testing %s/.git/ ", baseUrl)
	code, body, err = c.Get(nil, utils.Url(baseUrl, ".git/"))
	fmt.Printf("[%d]\n", code)
	if err != nil {
		return err
	}

	if code == 200 && utils.IsHtml(body) {
		indexedFiles, err := utils.GetIndexedFiles(body)
		if err != nil {
			return err
		}
		if utils.StringsContain(indexedFiles, "HEAD") {
			fmt.Println("[-] Fetching .git recursively")
			var wg sync.WaitGroup
			if err := DownloadRecursively(utils.Url(baseUrl, ".git/"), utils.Url(baseDir, ".git/"), indexedFiles, &wg); err != nil {
				return err
			}
			wg.Wait()
			fmt.Println("[-] Running git checkout .")
			cmd := exec.Command("git", "checkout", ".")
			cmd.Dir = baseDir
			return cmd.Run()
		}
	}
	return nil
}
