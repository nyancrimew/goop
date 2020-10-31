package goop

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/deletescape/goop/internal/utils"
	"github.com/deletescape/goop/internal/workers"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
	"github.com/valyala/fasthttp"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var c = &fasthttp.Client{
	MaxConnsPerHost: utils.MaxInt(maxConcurrency+250, fasthttp.DefaultMaxConnsPerHost),
	TLSConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
	NoDefaultUserAgentHeader: true,
	MaxConnWaitTimeout:       10 * time.Second,
}

func Clone(u, dir string, force bool) error {
	baseUrl := strings.TrimSuffix(u, "/")
	baseUrl = strings.TrimSuffix(baseUrl, "/HEAD")
	baseUrl = strings.TrimSuffix(baseUrl, "/.git")
	parsed, err := url.Parse(baseUrl)
	if err != nil {
		return err
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "http"
	}
	baseUrl = parsed.String()
	parsed, err = url.Parse(baseUrl)
	if err != nil {
		return err
	}
	baseDir := dir
	if baseDir == "" {
		baseDir = parsed.Host
	}

	if !utils.Exists(baseDir) {
		if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
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
		if force {
			if err := os.RemoveAll(baseDir); err != nil {
				return err
			}
			if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
				return err
			}
		} else {
			return fmt.Errorf("%s is not empty", baseDir)
		}
	}
	return FetchGit(baseUrl, baseDir)
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
			jobs := make(chan string)
			wg := sync.WaitGroup{}
			for w := 1; w <= maxConcurrency; w++ {
				go workers.RecursiveDownloadWorker(c, jobs, baseUrl, baseDir, &wg)
			}
			for _, f := range indexedFiles {
				// TODO: add support for non top level git repos
				jobs <- utils.Url(".git", f)
			}
			wg.Wait()
			close(jobs)
			fmt.Println("[-] Running git checkout .")
			cmd := exec.Command("git", "checkout", ".")
			cmd.Dir = baseDir
			return cmd.Run()
		}
	}

	fmt.Println("[-] Fetching common files")
	jobs := make(chan string)
	wg := sync.WaitGroup{}
	for w := 1; w <= utils.MinInt(maxConcurrency, len(commonFiles)); w++ {
		go workers.DownloadWorker(c, jobs, baseUrl, baseDir, &wg)
	}
	for _, f := range commonFiles {
		jobs <- f
	}
	close(jobs)
	wg.Wait()

	fmt.Println("[-] Finding refs")
	jobs = make(chan string)
	wg = sync.WaitGroup{}
	for w := 1; w <= maxConcurrency; w++ {
		go workers.FindRefWorker(c, jobs, baseUrl, baseDir, &wg)
	}
	for _, ref := range commonRefs {
		jobs <- ref
	}
	wg.Wait()
	close(jobs)

	fmt.Println("[-] Finding packs")
	infoPacksPath := utils.Url(baseDir, ".git/objects/info/packs")
	if utils.Exists(infoPacksPath) {
		fmt.Println("exists")
		infoPacks, err := ioutil.ReadFile(infoPacksPath)
		if err != nil {
			return err
		}
		hashes := packRegex.FindAll(infoPacks, -1)
		jobs := make(chan string)
		wg := sync.WaitGroup{}
		for w := 1; w <= utils.MinInt(maxConcurrency, len(hashes)); w++ {
			go workers.DownloadWorker(c, jobs, baseUrl, baseDir, &wg)
		}
		for _, sha1 := range hashes {
			jobs <- fmt.Sprintf("./git/objects/pack/pack-%s.idx", sha1)
			jobs <- fmt.Sprintf("./git/objects/pack/pack-%s.pack", sha1)
		}
		close(jobs)
		wg.Wait()
	}

	fmt.Println("[-] Finding objects")
	objs := make(map[string]bool) // object "set"
	//var packed_objs [][]byte

	files := []string{
		utils.Url(baseDir, ".git/packed-refs"),
		utils.Url(baseDir, ".git/info/refs"),
		utils.Url(baseDir, ".git/FETCH_HEAD"),
		utils.Url(baseDir, ".git/ORIG_HEAD"),
	}

	if err := filepath.Walk(utils.Url(baseDir, ".git/refs"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := filepath.Walk(utils.Url(baseDir, ".git/logs"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		return err
	}

	for _, f := range files {
		if !utils.Exists(f) {
			continue
		}

		content, err := ioutil.ReadFile(f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return err
		}

		for _, obj := range objRegex.FindAll(content, -1) {
			objs[strings.TrimSpace(string(obj))] = true
		}
	}

	indexPath := utils.Url(baseDir, ".git/index")
	if utils.Exists(indexPath) {
		f, err := os.Open(indexPath)
		if err != nil {
			return err
		}
		defer f.Close()
		var idx index.Index
		decoder := index.NewDecoder(f)
		if err := decoder.Decode(&idx); err != nil {
			return err
		}
		for _, entry := range idx.Entries {
			objs[entry.Hash.String()] = true
		}
	}

	storage := filesystem.NewObjectStorage(dotgit.New(osfs.New(utils.Url(baseDir, ".git"))), &cache.ObjectLRU{MaxSize: 256})
	if err := storage.ForEachObjectHash(func(hash plumbing.Hash) error {
		objs[hash.String()] = true
		encObj, err := storage.EncodedObject(plumbing.AnyObject, hash)
		if err != nil {
			return fmt.Errorf("error: %s\n", err)

		}
		decObj, err := object.DecodeObject(storage, encObj)
		if err != nil {
			return fmt.Errorf("error: %s\n", err)
		}
		for _, hash := range utils.GetReferencedHashes(decObj) {
			objs[hash] = true
		}
		return nil
	}); err != nil {
		return err
	}
	// TODO: find more objects to fetch in pack files and remove packed objects from list of objects to be fetched
	/*for _, pack := range storage.ObjectPacks() {
		storage.IterEncodedObjects()
	}*/

	fmt.Println("[-] Fetching objects")
	jobs = make(chan string)
	wg = sync.WaitGroup{}
	for w := 1; w <= maxConcurrency; w++ {
		go workers.FindObjectsWorker(c, jobs, baseUrl, baseDir, &wg, storage)
	}
	for obj := range objs {
		jobs <- obj
	}
	wg.Wait()
	close(jobs)

	fmt.Println("[-] Running git checkout .")
	cmd := exec.Command("git", "checkout", ".")
	cmd.Dir = baseDir
	return cmd.Run()
}
