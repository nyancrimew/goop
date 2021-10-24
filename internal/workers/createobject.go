package workers

import (
	"io/ioutil"
	"os"

	"github.com/deletescape/goop/internal/jobtracker"
	"github.com/deletescape/goop/internal/utils"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/index"
	"github.com/go-git/go-git/v5/storage/filesystem"
	"github.com/phuslu/log"
)

type CreateObjectContext struct {
	BaseDir string
	Storage *filesystem.ObjectStorage
	Index   *index.Index
}

func CreateObjectWorker(jt *jobtracker.JobTracker, f string, context jobtracker.Context) {
	c := context.(CreateObjectContext)

	fp := utils.Url(c.BaseDir, f)

	entry, err := c.Index.Entry(f)
	if err != nil {
		log.Error().Str("file", f).Err(err).Msg("file is not in index")
		return
	}

	fMode, err := entry.Mode.ToOSFileMode()
	if err != nil {
		log.Warn().Str("file", f).Err(err).Msg("failed to set filemode")
	} else {
		os.Chmod(fp, fMode)
	}
	os.Chown(fp, int(entry.UID), int(entry.GID))
	os.Chtimes(fp, entry.ModifiedAt, entry.ModifiedAt)
	//log.Info().Str("file", f).Msg("updated from index")

	content, err := ioutil.ReadFile(fp)
	if err != nil {
		log.Error().Str("file", f).Err(err).Msg("failed to read file")
		return
	}

	hash := plumbing.ComputeHash(plumbing.BlobObject, content)
	if entry.Hash != hash {
		log.Warn().Str("file", f).Msg("hash does not match hash in index, skipping object creation")
		return
	}

	obj := c.Storage.NewEncodedObject()
	obj.SetSize(int64(len(content)))
	obj.SetType(plumbing.BlobObject)

	ow, err := obj.Writer()
	if err != nil {
		log.Error().Str("file", f).Err(err).Msg("failed to create object writer")
		return
	}
	defer ow.Close()
	ow.Write(content)

	_, err = c.Storage.SetEncodedObject(obj)
	if err != nil {
		log.Error().Str("file", f).Err(err).Msg("failed to create object")
		return
	}
	//log.Info().Str("file", f).Msg("object created")
}
