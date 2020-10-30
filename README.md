# goop

Yet another tool to dump a git repository from a website. Code structure and console outputs heavily inspired by [arthaud/git-dumper](https://github.com/arthaud/git-dumper).

## Usage
```bash
usage: goop [flags] url [dir]

Flags:
   -f, --force   overrides DIR if it already exists
   -h, --help    help for goop
```

### Example
```bash
$ goop example.com
```

## Installation

```bash
go get -u github.com/deletescape/goop
```

## How does it work?

The tool will first check if directory listing is available. If it is, then it will just recursively download the .git directory (what you would do with `wget`).

If directory listing is not available, it will use several methods to find as many files as possible. Step by step, git-dumper will:
* Fetch all common files (`.gitignore`, `.git/HEAD`, `.git/index`, etc.);
* Find as many refs as possible (such as `refs/heads/master`, `refs/remotes/origin/HEAD`, etc.) by analyzing `.git/HEAD`, `.git/logs/HEAD`, `.git/config`, `.git/packed-refs` and so on;
* Find as many objects (sha1) as possible by analyzing `.git/packed-refs`, `.git/index`, `.git/refs/*` and `.git/logs/*`;
* Fetch all objects recursively, analyzing each commits to find their parents;
* Run `git checkout .` to recover the current working tree
