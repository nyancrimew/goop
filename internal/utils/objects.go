package utils

import "github.com/go-git/go-git/v5/plumbing/object"

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
