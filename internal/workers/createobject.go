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

func CreateObjectWorker(baseDir string, jt *jobtracker.JobTracker, storage *filesystem.ObjectStorage, index *index.Index) {
	for {
		select {
		case file := <-jt.Queue:
			createObjWork(baseDir, file, jt, storage, index)
		default:
			if !jt.HasWork() {
				return
			}
			jt.Nap()
		}
	}
}

func createObjWork(baseDir, f string, jt *jobtracker.JobTracker, storage *filesystem.ObjectStorage, idx *index.Index) {
	jt.StartWork()
	defer jt.EndWork()

	fp := utils.Url(baseDir, f)

	entry, err := idx.Entry(f)
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

	obj := storage.NewEncodedObject()
	obj.SetSize(int64(len(content)))
	obj.SetType(plumbing.BlobObject)

	ow, err := obj.Writer()
	if err != nil {
		log.Error().Str("file", f).Err(err).Msg("failed to create object writer")
		return
	}
	defer ow.Close()
	ow.Write(content)

	_, err = storage.SetEncodedObject(obj)
	if err != nil {
		log.Error().Str("file", f).Err(err).Msg("failed to create object")
		return
	}
	//log.Info().Str("file", f).Msg("object created")
}
