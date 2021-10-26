package goop

import (
	"bufio"
	"bytes"
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/deletescape/goop/internal/utils"
	"github.com/deletescape/goop/internal/workers"
	"github.com/deletescape/jobtracker"
	"github.com/go-git/go-billy/v5/osfs"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/cache"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/go-git/go-git/v5/storage/filesystem/dotgit"
	"github.com/phuslu/log"
	"github.com/valyala/fasthttp"
	"github.com/valyala/fasthttp/fasthttpproxy"
)

func proxyFromEnv() fasthttp.DialFunc {
	allProxy, okAll := os.LookupEnv("all_proxy")
	httpProxy, okHttp := os.LookupEnv("http_proxy")
	httpsProxy, okHttps := os.LookupEnv("https_proxy")

	uriToDial := func(u string) fasthttp.DialFunc {
		pUri, err := url.Parse(u)
		if err != nil {
			panic(err) // TODO: uh, handle better
		}
		if pUri.Scheme == "socks5" {
			return fasthttpproxy.FasthttpSocksDialer(pUri.Host)
		}
		return fasthttpproxy.FasthttpHTTPDialer(pUri.Host) // this probly doesnt work for proxys with auth rn
	}

	if okAll {
		return uriToDial(allProxy)
	}
	if okHttp {
		return uriToDial(httpProxy)
	}
	if okHttps {
		return uriToDial(httpsProxy)
	}

	return nil
}

var c = &fasthttp.Client{
	Name:            "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/85.0.4183.102 Safari/537.36",
	MaxConnsPerHost: utils.MaxInt(maxConcurrency+250, fasthttp.DefaultMaxConnsPerHost),
	TLSConfig: &tls.Config{
		InsecureSkipVerify: true,
	},
	NoDefaultUserAgentHeader: true,
	MaxConnWaitTimeout:       10 * time.Second,
	Dial:                     proxyFromEnv(),
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
				log.Error().Str("uri", u).Err(err).Msg("couldn't parse uri")
				continue
			}
			dir = utils.Url(dir, parsed.Host)
		}
		log.Info().Str("target", u).Str("dir", dir).Bool("force", force).Bool("keep", keep).Msg("starting download")
		if err := Clone(u, dir, force, keep); err != nil {
			log.Error().Str("target", u).Str("dir", dir).Bool("force", force).Bool("keep", keep).Msg("download failed")
		}
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

	if utils.Exists(baseDir) {
		if !utils.IsFolder(baseDir) {
			return fmt.Errorf("%s is not a directory", baseDir)
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
			} else if !keep {
				return fmt.Errorf("%s is not empty", baseDir)
			}
		}
	}

	return FetchGit(baseUrl, baseDir)
}

func FetchGit(baseUrl, baseDir string) error {
	log.Info().Str("base", baseUrl).Msg("testing for .git/HEAD")
	code, body, err := c.Get(nil, utils.Url(baseUrl, ".git/HEAD"))
	if err != nil {
		return err
	}

	if code != 200 {
		log.Warn().Str("base", baseUrl).Int("code", code).Msg(".git/HEAD doesn't appear to exist, clone will most likely fail")
	} else if !bytes.HasPrefix(body, refPrefix) {
		log.Warn().Str("base", baseUrl).Int("code", code).Msg(".git/HEAD doesn't appear to be a git HEAD file, clone will most likely fail")
	}

	log.Info().Str("base", baseUrl).Msg("testing if recursive download is possible")
	code, body, err = c.Get(body, utils.Url(baseUrl, ".git/"))
	if err != nil {
		if utils.IgnoreError(err) {
			log.Error().Str("base", baseUrl).Int("code", code).Err(err)
		} else {
			return err
		}
	}

	if code == 200 && utils.IsHtml(body) {
		lnk, _ := url.Parse(utils.Url(baseUrl, ".git/"))
		indexedFiles, err := utils.GetIndexedFiles(body, lnk.Path)
		if err != nil {
			return err
		}
		if utils.StringsContain(indexedFiles, "HEAD") {
			log.Info().Str("base", baseUrl).Msg("fetching .git/ recursively")
			jt := jobtracker.NewJobTracker(workers.RecursiveDownloadWorker, maxConcurrency, jobtracker.DefaultNapper)
			jt.AddJobs(indexedFiles...)
			jt.StartAndWait(&workers.RecursiveDownloadContext{C: c, BaseUrl: baseUrl, BaseDir: baseDir}, true)

			if err := checkout(baseDir); err != nil {
				log.Error().Str("dir", baseDir).Err(err).Msg("failed to checkout")
			}
			if err := fetchIgnored(baseDir, baseUrl); err != nil {
				return err
			}
		}
	}

	log.Info().Str("base", baseUrl).Msg("fetching common files")
	jt := jobtracker.NewJobTracker(workers.DownloadWorker, maxConcurrency, jobtracker.DefaultNapper)
	jt.AddJobs(commonFiles...)
	jt.StartAndWait(workers.DownloadContext{C: c, BaseDir: baseDir, BaseUrl: baseUrl}, false)

	log.Info().Str("base", baseUrl).Msg("finding refs")
	jt = jobtracker.NewJobTracker(workers.FindRefWorker, maxConcurrency, jobtracker.DefaultNapper)
	jt.AddJobs(commonRefs...)
	jt.StartAndWait(workers.FindRefContext{C: c, BaseUrl: baseUrl, BaseDir: baseDir}, true)

	log.Info().Str("base", baseUrl).Msg("finding packs")
	infoPacksPath := utils.Url(baseDir, ".git/objects/info/packs")
	if utils.Exists(infoPacksPath) {
		infoPacks, err := ioutil.ReadFile(infoPacksPath)
		if err != nil {
			return err
		}
		hashes := packRegex.FindAllSubmatch(infoPacks, -1)
		jt = jobtracker.NewJobTracker(workers.DownloadWorker, maxConcurrency, jobtracker.DefaultNapper)
		for _, sha1 := range hashes {
			jt.AddJobs(
				fmt.Sprintf(".git/objects/pack/pack-%s.idx", sha1[1]),
				fmt.Sprintf(".git/objects/pack/pack-%s.pack", sha1[1]),
				fmt.Sprintf(".git/objects/pack/pack-%s.rev", sha1[1]),
			)
		}
		jt.StartAndWait(workers.DownloadContext{C: c, BaseUrl: baseUrl, BaseDir: baseDir}, false)
	}

	log.Info().Str("base", baseUrl).Msg("finding objects")
	objs := make(map[string]bool) // object "set"
	//var packed_objs [][]byte

	files := []string{
		utils.Url(baseDir, ".git/packed-refs"),
		utils.Url(baseDir, ".git/info/refs"),
		utils.Url(baseDir, ".git/info/grafts"),
		// utils.Url(baseDir, ".git/info/sparse-checkout"), // TODO: ?
		utils.Url(baseDir, ".git/FETCH_HEAD"),
		utils.Url(baseDir, ".git/ORIG_HEAD"),
		utils.Url(baseDir, ".git/HEAD"),
		utils.Url(baseDir, ".git/objects/loose-object-idx"), // TODO: is this even a text file?
		utils.Url(baseDir, ".git/objects/info/commit-graphs/commit-graph-chain"),
		utils.Url(baseDir, ".git/objects/info/alternates"),
		utils.Url(baseDir, ".git/objects/info/http-alternates"),
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
		refLogPrefix := utils.Url(gitLogsDir, "refs") + "/"
		if err := filepath.Walk(gitLogsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				files = append(files, path)

				if strings.HasPrefix(path, refLogPrefix) {
					refName := strings.TrimPrefix(path, refLogPrefix)
					filePath := utils.Url(gitRefsDir, refName)
					if !utils.Exists(filePath) {
						log.Info().Str("dir", baseDir).Str("ref", refName).Msg("generating ref file")

						content, err := ioutil.ReadFile(path)
						if err != nil {
							log.Error().Str("dir", baseDir).Str("ref", refName).Err(err).Msg("couldn't read reflog file")
							return nil
						}

						// Find the last reflog entry and extract the obj hash and write that to the ref file
						logObjs := refLogRegex.FindAllSubmatch(content, -1)
						lastEntryObj := logObjs[len(logObjs)-1][1]

						if err := utils.CreateParentFolders(filePath); err != nil {
							log.Error().Str("file", filePath).Err(err).Msg("couldn't create parent directories")
							return nil
						}

						if err := ioutil.WriteFile(filePath, lastEntryObj, os.ModePerm); err != nil {
							log.Error().Str("file", filePath).Err(err).Msg("couldn't write to file")
						}
					}
				}
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
			log.Error().Str("file", f).Err(err).Msg("couldn't read reflog file")
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
			log.Error().Str("dir", baseDir).Err(err).Msg("couldn't decode git index")
		}
		for _, entry := range idx.Entries {
			objs[entry.Hash.String()] = true
		}
	}

	objStorage := filesystem.NewObjectStorage(dotgit.New(osfs.New(utils.Url(baseDir, ".git"))), &cache.ObjectLRU{MaxSize: 256})
	if err := objStorage.ForEachObjectHash(func(hash plumbing.Hash) error {
		objs[hash.String()] = true
		encObj, err := objStorage.EncodedObject(plumbing.AnyObject, hash)
		if err != nil {
			return err

		}
		decObj, err := object.DecodeObject(objStorage, encObj)
		if err != nil {
			return err
		}
		for _, hash := range utils.GetReferencedHashes(decObj) {
			objs[hash] = true
		}
		return nil
	}); err != nil {
		log.Error().Str("dir", baseDir).Err(err).Msg("error while processing object files")
	}
	// TODO: find more objects to fetch in pack files and remove packed objects from list of objects to be fetched
	/*for _, pack := range storage.ObjectPacks() {
		storage.IterEncodedObjects()
	}*/

	// TODO: grab object hashes from commit graphs

	log.Info().Str("base", baseUrl).Msg("fetching objects")
	jt = jobtracker.NewJobTracker(workers.FindObjectsWorker, maxConcurrency, jobtracker.DefaultNapper)
	for obj := range objs {
		jt.AddJob(obj)
	}
	jt.StartAndWait(workers.FindObjectsContext{C: c, BaseUrl: baseUrl, BaseDir: baseDir, Storage: objStorage}, true)

	// exit early if we haven't managed to dump anything
	if !utils.Exists(baseDir) {
		return nil
	}

	fetchMissing(baseDir, baseUrl, objStorage)

	// TODO: disable lfs in checkout (for now lfs support depends on lfs NOT being setup on the system you use goop on)
	if err := checkout(baseDir); err != nil {
		log.Error().Str("dir", baseDir).Err(err).Msg("failed to checkout")
	}

	// <fetch lfs objects and manually check them out>
	fetchLfs(baseDir, baseUrl)

	if err := fetchIgnored(baseDir, baseUrl); err != nil {
		return err
	}

	return nil
}

func checkout(baseDir string) error {
	log.Info().Str("dir", baseDir).Msg("running git checkout .")
	cmd := exec.Command("git", "checkout", ".")
	cmd.Dir = baseDir
	return cmd.Run()
}

func fetchLfs(baseDir, baseUrl string) {
	attrPath := utils.Url(baseDir, ".gitattributes")
	if utils.Exists(attrPath) {
		log.Info().Str("dir", baseDir).Msg("attempting to fetch potential git lfs objects")
		f, err := os.Open(attrPath)
		if err != nil {
			log.Error().Str("dir", baseDir).Err(err).Msg("couldn't read git attributes")
			return
		}
		defer f.Close()

		var globalFilters []string
		var filters []string
		var files []string
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.Contains(line, "filter=lfs") {
				pattern := strings.SplitN(line, " ", 2)[0]
				if strings.ContainsRune(pattern, '*') {
					if strings.ContainsRune(pattern, '/') {
						globalFilters = append(globalFilters, pattern)
					} else {
						filters = append(filters, pattern)
					}
				} else {
					files = append(files, pattern)
				}
			}
		}
		if err := scanner.Err(); err != nil {
			log.Error().Str("dir", baseDir).Err(err).Msg("error while parsing git attributes file")
		}

		var hashes []string
		readStub := func(fp string) {
			f, err := os.Open(fp)
			if err != nil {
				log.Error().Str("file", fp).Err(err).Msg("couldn't open lfs stub file")
				return
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				fmt.Println(line)
				if strings.HasPrefix(line, "oid ") {
					hash := strings.SplitN(line, " ", 2)[1]
					hash = strings.SplitN(hash, ":", 2)[1]
					hashes = append(hashes, hash)
					break
				}
			}
			if err := scanner.Err(); err != nil {
				log.Error().Str("file", fp).Err(err).Msg("error while parsing lfs stub file")
			}
		}

		for _, file := range files {
			fp := utils.Url(baseDir, file)
			if utils.Exists(fp) {
				readStub(fp)
			}
		}

		err = filepath.Walk(baseDir,
			func(path string, info os.FileInfo, err error) error {
				if err != nil {
					return err
				}
				for _, filter := range filters {
					match, err := filepath.Match(filter, filepath.Base(path))
					if err != nil {
						log.Error().Str("dir", baseDir).Str("filter", filter).Err(err).Msg("failed to apply filter")
						continue
					}
					if match {
						readStub(path)
					}
				}
				return nil
			})
		if err != nil {
			log.Error().Str("dir", baseDir).Err(err).Msg("error while testing git lfs filters")
		}

		// TODO: global filters

		jt := jobtracker.NewJobTracker(workers.DownloadWorker, maxConcurrency, jobtracker.DefaultNapper)
		for _, hash := range hashes {
			jt.AddJob(fmt.Sprintf(".git/lfs/objects/%s/%s/%s", hash[:2], hash[2:4], hash))
		}
		jt.StartAndWait(workers.DownloadContext{C: c, BaseUrl: baseUrl, BaseDir: baseDir}, false)
	}
}

// Iterate over index to find missing files
func fetchMissing(baseDir, baseUrl string, objStorage *filesystem.ObjectStorage) {
	indexPath := utils.Url(baseDir, ".git/index")
	if utils.Exists(indexPath) {
		log.Info().Str("base", baseUrl).Str("dir", baseDir).Msg("attempting to fetch potentially missing files")

		var missingFiles []string
		var idx index.Index
		f, err := os.Open(indexPath)
		if err != nil {
			log.Error().Str("dir", baseDir).Err(err).Msg("couldn't read git index")
			return
		}
		defer f.Close()
		decoder := index.NewDecoder(f)
		if err := decoder.Decode(&idx); err != nil {
			log.Error().Str("dir", baseDir).Err(err).Msg("couldn't decode git index")
			return
		} else {
			jt := jobtracker.NewJobTracker(workers.DownloadWorker, maxConcurrency, jobtracker.DefaultNapper)
			for _, entry := range idx.Entries {
				if !strings.HasSuffix(entry.Name, ".php") && !utils.Exists(utils.Url(baseDir, fmt.Sprintf(".git/objects/%s/%s", entry.Hash.String()[:2], entry.Hash.String()[2:]))) {
					missingFiles = append(missingFiles, entry.Name)
					jt.AddJob(entry.Name)
				}
			}
			jt.StartAndWait(workers.DownloadContext{C: c, BaseUrl: baseUrl, BaseDir: baseDir, AllowHtml: true, AlllowEmpty: true}, false)

			jt = jobtracker.NewJobTracker(workers.CreateObjectWorker, maxConcurrency, jobtracker.DefaultNapper)
			for _, f := range missingFiles {
				if utils.Exists(utils.Url(baseDir, f)) {
					jt.AddJob(f)
				}
			}
			jt.StartAndWait(workers.CreateObjectContext{BaseDir: baseDir, Storage: objStorage, Index: &idx}, false)
		}
	}
}

func fetchIgnored(baseDir, baseUrl string) error {
	ignorePath := utils.Url(baseDir, ".gitignore")
	if utils.Exists(ignorePath) {
		log.Info().Str("base", baseDir).Msg("atempting to fetch ignored files")

		ignoreFile, err := os.Open(ignorePath)
		if err != nil {
			return err
		}
		defer ignoreFile.Close()

		jt := jobtracker.NewJobTracker(workers.DownloadWorker, maxConcurrency, jobtracker.DefaultNapper)

		scanner := bufio.NewScanner(ignoreFile)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			commentStrip := strings.SplitN(line, "#", 1)
			line = commentStrip[0]
			if line == "" || strings.HasPrefix(line, "!") || strings.HasSuffix(line, "/") || strings.ContainsRune(line, '*') || strings.HasSuffix(line, ".php") {
				continue
			}
			jt.AddJob(line)
		}

		if err := scanner.Err(); err != nil {
			return err
		}

		jt.StartAndWait(workers.DownloadContext{C: c, BaseUrl: baseUrl, BaseDir: baseDir, AllowHtml: true, AlllowEmpty: true}, false)
	}
	return nil
}
