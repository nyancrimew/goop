package goop

import "regexp"

const maxConcurrency = 40

var refPrefix = []byte{'r', 'e', 'f', ':'}
var (
	packRegex   = regexp.MustCompile(`(?m)pack-([a-f0-9]{40})\.pack`)
	objRegex    = regexp.MustCompile(`(?m)(^|\s)([a-f0-9]{40})($|\s)`)
	refLogRegex = regexp.MustCompile(`(?m)^(?:[a-f0-9]{40}) ([a-f0-9]{40}) .*$`)
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
		".git/objects/info/alternates",                       // TODO: parse and process
		".git/objects/info/http-alternates",                  // TODO: parse and process
		".git/objects/info/commit-graph",                     // TODO: parse for object hashes
		".git/objects/info/commit-graphs/commit-graph-chain", // TODO: read file and fetch mentioned graph files too, then parse those for object hashes
		".git/info/grafts",                                   // TODO: parse and process
		".git/info/attributes",                               // TODO: can lfs filters be in here?
		".git/info/sparse-checkout",                          // TODO: parse and process
		".git/objects/loose-object-idx",                      // TODO: parse and process
		".git/objects/pack/multi-pack-index",                 // TODO: parse and process
	}
	commonRefs = []string{
		".git/FETCH_HEAD",
		".git/HEAD",
		".git/ORIG_HEAD",
		".git/config",
		".git/config.worktree",
		".git/info/refs",
		".git/logs/HEAD",
		".git/logs/refs/heads/master",
		".git/logs/refs/heads/main",
		".git/logs/refs/heads/dev",
		".git/logs/refs/heads/develop",
		".git/logs/refs/tags/alpha",
		".git/logs/refs/tags/beta",
		".git/logs/refs/tags/stable",
		".git/logs/refs/tags/release",
		".git/logs/refs/tags/1.0",
		".git/logs/refs/tags/1.0.0",
		".git/logs/refs/tags/2.0",
		".git/logs/refs/tags/2.0.0",
		".git/logs/refs/tags/v1.0",
		".git/logs/refs/tags/v1.0.0",
		".git/logs/refs/tags/v2.0",
		".git/logs/refs/tags/v2.0.0",
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
		".git/refs/tags/alpha",
		".git/refs/tags/beta",
		".git/refs/tags/stable",
		".git/refs/tags/release",
		".git/refs/tags/1.0",
		".git/refs/tags/1.0.0",
		".git/refs/tags/2.0",
		".git/refs/tags/2.0.0",
		".git/refs/tags/v1.0",
		".git/refs/tags/v1.0.0",
		".git/refs/tags/v2.0",
		".git/refs/tags/v2.0.0",
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
