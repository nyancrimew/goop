package goop

import (
	"bufio"
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
	Name:            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.102 Safari/537.36",
	MaxConnsPerHost: utils.MaxInt(maxConcurrency+250, fasthttp.DefaultMaxConnsPerHost),
	TLSConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
	NoDefaultUserAgentHeader: true,
	MaxConnWaitTimeout:       10 * time.Second,
}

var wg sync.WaitGroup

func createQueue(scale int) chan string {
	wg = sync.WaitGroup{}
	return make(chan string, maxConcurrency*scale)
}

func waitForQueue(queue chan string) {
	wg.Wait()
	close(queue)
}

func CloneList(listFile, baseDir string, force, keep bool) error {
	lf, err := os.Open(listFile)
	if err != nil {
		return err
	}
	defer lf.Close()

	listScan := bufio.NewScanner(lf)
	for listScan.Scan() {
		u := listScan.Text()
		if u == "" {
			continue
		}
		dir := baseDir
		if dir != "" {
			parsed, err := url.Parse(u)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				continue
			}
			dir = utils.Url(dir, parsed.Host)
		}
		fmt.Printf("[-] Downloading %s to %s\n", u, dir)
		if err := Clone(u, dir, force, keep); err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
		}
		fmt.Println()
		fmt.Println()
	}
	return nil
}

func Clone(u, dir string, force, keep bool) error {
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
		if force || keep {
			if !keep {
				if err := os.RemoveAll(baseDir); err != nil {
					return err
				}
				if err := os.MkdirAll(baseDir, os.ModePerm); err != nil {
					return err
				}
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
		fmt.Fprintf(os.Stderr, "error: %s/.git/HEAD does not exist", baseUrl)
		//return fmt.Errorf("error: %s/.git/HEAD does not exist", baseUrl)
	} else if !bytes.HasPrefix(body, refPrefix) {
		fmt.Fprintf(os.Stderr, "error: %s/.git/HEAD is not a git HEAD file", baseUrl)
		//return fmt.Errorf("error: %s/.git/HEAD is not a git HEAD file", baseUrl)
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
			queue := createQueue(1000)
			for w := 1; w <= maxConcurrency; w++ {
				go workers.RecursiveDownloadWorker(c, queue, baseUrl, baseDir, &wg)
			}
			for _, f := range indexedFiles {
				// TODO: add support for non top level git repos
				queue <- utils.Url(".git", f)
			}
			waitForQueue(queue)
			fmt.Println("[-] Running git checkout .")
			cmd := exec.Command("git", "checkout", ".")
			cmd.Dir = baseDir
			return cmd.Run()
		}
	}

	fmt.Println("[-] Fetching common files")
	queue := createQueue(len(commonFiles))
	for w := 1; w <= utils.MinInt(maxConcurrency, len(commonFiles)); w++ {
		go workers.DownloadWorker(c, queue, baseUrl, baseDir, &wg)
	}
	for _, f := range commonFiles {
		queue <- f
	}
	close(queue)
	wg.Wait()

	fmt.Println("[-] Finding refs")
	queue = createQueue(100)
	for w := 1; w <= maxConcurrency; w++ {
		go workers.FindRefWorker(c, queue, baseUrl, baseDir, &wg)
	}
	for _, ref := range commonRefs {
		queue <- ref
	}
	waitForQueue(queue)

	fmt.Println("[-] Finding packs")
	infoPacksPath := utils.Url(baseDir, ".git/objects/info/packs")
	if utils.Exists(infoPacksPath) {
		infoPacks, err := ioutil.ReadFile(infoPacksPath)
		if err != nil {
			return err
		}
		hashes := packRegex.FindAllSubmatch(infoPacks, -1)
		queue = createQueue(len(hashes) * 3)
		for w := 1; w <= utils.MinInt(maxConcurrency, len(hashes)); w++ {
			go workers.DownloadWorker(c, queue, baseUrl, baseDir, &wg)
		}
		for _, sha1 := range hashes {
			queue <- fmt.Sprintf(".git/objects/pack/pack-%s.idx", sha1[1])
			queue <- fmt.Sprintf(".git/objects/pack/pack-%s.pack", sha1[1])
		}
		close(queue)
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

	gitRefsDir := utils.Url(baseDir, ".git/refs")
	if utils.Exists(gitRefsDir) {
		if err := filepath.Walk(gitRefsDir, func(path string, info os.FileInfo, err error) error {
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
	}
	gitLogsDir := utils.Url(baseDir, ".git/logs")
	if utils.Exists(gitLogsDir) {
		if err := filepath.Walk(gitLogsDir, func(path string, info os.FileInfo, err error) error {
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
	queue = createQueue(2000)
	for w := 1; w <= maxConcurrency; w++ {
		go workers.FindObjectsWorker(c, queue, baseUrl, baseDir, &wg, storage)
	}
	for obj := range objs {
		queue <- obj
	}
	waitForQueue(queue)

	fmt.Println("[-] Running git checkout .")
	cmd := exec.Command("git", "checkout", ".")
	cmd.Dir = baseDir
	return cmd.Run()
}
