package goop

import "regexp"

const maxConcurrency = 30

var refPrefix = []byte{'r', 'e', 'f', ':'}
var phpSuffix = []byte{'.', 'p', 'h', 'p'}
var (
	packRegex = regexp.MustCompile(`(?m)pack-([a-f0-9]{40})\.pack`)
	objRegex  = regexp.MustCompile(`(?m)(^|\s)([a-f0-9]{40})($|\s)`)
	refLogRegex  = regexp.MustCompile(`(?m)^(?:[a-f0-9]{40}) ([a-f0-9]{40}) .*$`)
	stdErrRegex = regexp.MustCompile(`error: unable to read sha1 file of (.+?) \(.*`)
	statusRegex = regexp.MustCompile(`deleted: {4}(.+)`)
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