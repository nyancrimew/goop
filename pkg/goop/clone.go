package goop

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"github.com/deletescape/goop/internal/utils"
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
	"regexp"
	"strings"
	"sync"
)

// TODO: implement limiter
const maxConcurrency = 500

var c = &fasthttp.Client{
	MaxConnsPerHost: maxConcurrency + 250,
	TLSConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
}

var refPrefix = []byte{'r', 'e', 'f', ':'}
var (
	refRegex  = regexp.MustCompile(`(?m)(refs(/[a-zA-Z0-9\-\.\_\*]+)+)`)
	packRegex = regexp.MustCompile(`(?m)pack-([a-f0-9]{40})\.pack`)
	objRegex  = regexp.MustCompile(`(?m)(^|\s)([a-f0-9]{40})($|\s)`)
)
var (
	commonFiles = []string{
		".gitignore",
		".gitattributes",
		".gitmodules",
		".env",
		".git/COMMIT_EDITMSG",
		".git/description",
		".git/hooks/applypatch-msg.sample",
		".git/hooks/applypatch-msg.sample",
		".git/hooks/applypatch-msg.sample",
		".git/hooks/commit-msg.sample",
		".git/hooks/post-commit.sample",
		".git/hooks/post-receive.sample",
		".git/hooks/post-update.sample",
		".git/hooks/pre-applypatch.sample",
		".git/hooks/pre-commit.sample",
		".git/hooks/pre-push.sample",
		".git/hooks/pre-rebase.sample",
		".git/hooks/pre-receive.sample",
		".git/hooks/prepare-commit-msg.sample",
		".git/hooks/update.sample",
		".git/index",
		".git/info/exclude",
		".git/objects/info/packs",
	}
	commonRefs = []string{
		".git/FETCH_HEAD",
		".git/HEAD",
		".git/ORIG_HEAD",
		".git/config",
		".git/info/refs",
		".git/logs/HEAD",
		".git/logs/refs/heads/master",
		".git/logs/refs/heads/main",
		".git/logs/refs/heads/dev",
		".git/logs/refs/heads/develop",
		".git/logs/refs/remotes/origin/HEAD",
		".git/logs/refs/remotes/origin/master",
		".git/logs/refs/remotes/origin/main",
		".git/logs/refs/remotes/origin/dev",
		".git/logs/refs/remotes/origin/develop",
		".git/logs/refs/stash",
		".git/packed-refs",
		".git/refs/heads/master",
		".git/refs/heads/main",
		".git/refs/heads/dev",
		".git/refs/heads/develop",
		".git/refs/remotes/origin/HEAD",
		".git/refs/remotes/origin/master",
		".git/refs/remotes/origin/main",
		".git/refs/remotes/origin/dev",
		".git/refs/remotes/origin/develop",
		".git/refs/stash",
		".git/refs/wip/wtree/refs/heads/master", //Magit
		".git/refs/wip/index/refs/heads/master", //Magit
	}
)

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

func DownloadRecursively(u, dir string, files []string, wg *sync.WaitGroup) {
	os.MkdirAll(dir, os.ModePerm)
	for _, f := range files {
		f := f
		uri := utils.Url(u, f)
		code, body, err := c.Get(nil, uri)
		fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return
		}
		if strings.HasSuffix(f, "/") {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if !utils.IsHtml(body) {
					fmt.Printf("warning: %s doesn't appear to be an index", uri)
					return
				}
				indexedFiles, err := utils.GetIndexedFiles(body)
				if err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					return
				}
				DownloadRecursively(uri, utils.Url(dir, f), indexedFiles, wg)
			}()
		} else {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if utils.IsHtml(body) {
					fmt.Printf("warning: %s doesn't appear to be a git file", uri)
					return
				}
				if err := utils.CreateParentFolders(utils.Url(dir, f)); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
					return
				}
				if err := ioutil.WriteFile(utils.Url(dir, f), body, os.ModePerm); err != nil {
					fmt.Fprintf(os.Stderr, "error: %s\n", err)
				}
			}()
		}
	}
}

func FindRefs(baseUrl, baseDir, path string, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		uri := utils.Url(baseUrl, path)
		code, body, err := c.Get(nil, uri)
		fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return
		}
		if code == 200 {
			if utils.IsHtml(body) {
				fmt.Printf("warning: %s appears to be an html file, skipping\n", uri)
				return
			}
			if err := utils.CreateParentFolders(utils.Url(baseDir, path)); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return
			}
			if err := ioutil.WriteFile(utils.Url(baseDir, path), body, os.ModePerm); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return
			}

			for _, ref := range refRegex.FindAll(body, -1) {
				FindRefs(baseUrl, baseDir, string(ref), wg)
			}
		}
	}()
}

func Download(baseUrl, baseDir, file string, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		uri := utils.Url(baseUrl, file)
		code, body, err := c.Get(nil, uri)
		fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return
		}
		if code == 200 {
			if utils.IsHtml(body) {
				fmt.Printf("warning: %s appears to be an html file, skipping\n", uri)
				return
			}
			if err := utils.CreateParentFolders(utils.Url(baseDir, file)); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return
			}
			if err := ioutil.WriteFile(utils.Url(baseDir, file), body, os.ModePerm); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
			}
		}
	}()
}

func GetReferencedHashes(obj object.Object) []string {
	var hashes []string
	switch o := obj.(type) {
	case *object.Commit:
		hashes = append(hashes, o.TreeHash.String())
		for _, p := range o.ParentHashes {
			hashes = append(hashes, p.String())
		}
	case *object.Tree:
		for _, e := range o.Entries {
			hashes = append(hashes, e.Hash.String())
		}
	case *object.Blob:
		// pass
	case *object.Tag:
		hashes = append(hashes, o.Target.String())
	}
	return hashes
}

// TODO: more dedupe stuff
func FindObjects(obj, baseUrl, baseDir string, storage *filesystem.ObjectStorage, wg *sync.WaitGroup) {
	wg.Add(1)
	go func() {
		defer wg.Done()
		file := fmt.Sprintf(".git/objects/%s/%s", obj[:2], obj[2:])
		uri := utils.Url(baseUrl, file)
		code, body, err := c.Get(nil, uri)
		fmt.Printf("[-] Fetching %s [%d]\n", uri, code)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %s\n", err)
			return
		}
		if code == 200 {
			if utils.IsHtml(body) {
				fmt.Printf("warning: %s appears to be an html file, skipping\n", uri)
				return
			}
			fullPath := utils.Url(baseDir, file)
			if err := utils.CreateParentFolders(fullPath); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return
			}
			if err := ioutil.WriteFile(fullPath, body, os.ModePerm); err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return
			}

			encObj, err := storage.EncodedObject(plumbing.AnyObject, plumbing.NewHash(obj))
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return
			}
			decObj, err := object.DecodeObject(storage, encObj)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %s\n", err)
				return
			}
			referencedHashes := GetReferencedHashes(decObj)
			for _, h := range referencedHashes {
				FindObjects(h, baseUrl, baseDir, storage, wg)
			}
		}
	}()
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
			wg := sync.WaitGroup{}
			DownloadRecursively(utils.Url(baseUrl, ".git/"), utils.Url(baseDir, ".git/"), indexedFiles, &wg)
			wg.Wait()
			fmt.Println("[-] Running git checkout .")
			cmd := exec.Command("git", "checkout", ".")
			cmd.Dir = baseDir
			return cmd.Run()
		}
	}

	fmt.Println("[-] Fetching common files")
	wg := sync.WaitGroup{}
	for _, f := range commonFiles {
		Download(baseUrl, baseDir, f, &wg)
	}
	wg.Wait()

	fmt.Println("[-] Finding refs")
	wg = sync.WaitGroup{}
	for _, ref := range commonRefs {
		FindRefs(baseUrl, baseDir, ref, &wg)
	}
	wg.Wait()

	fmt.Println("[-] Finding packs")
	infoPacksPath := utils.Url(baseDir, ".git/objects/info/packs")
	if utils.Exists(infoPacksPath) {
		fmt.Println("exists")
		infoPacks, err := ioutil.ReadFile(infoPacksPath)
		if err != nil {
			return err
		}
		wg = sync.WaitGroup{}
		for _, sha1 := range packRegex.FindAll(infoPacks, -1) {
			Download(baseUrl, baseDir, fmt.Sprintf("./git/objects/pack/pack-%s.idx", sha1), &wg)
			Download(baseUrl, baseDir, fmt.Sprintf("./git/objects/pack/pack-%s.pack", sha1), &wg)
		}
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
		fmt.Println(f)
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
		defer f.Close()
		if err != nil {
			return err
		}
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
		for _, hash := range GetReferencedHashes(decObj) {
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
	wg = sync.WaitGroup{}
	for obj := range objs {
		FindObjects(obj, baseUrl, baseDir, storage, &wg)
	}
	wg.Wait()

	fmt.Println("[-] Running git checkout .")
	cmd := exec.Command("git", "checkout", ".")
	cmd.Dir = baseDir
	return cmd.Run()
}
