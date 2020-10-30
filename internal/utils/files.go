package utils

import (
	"io"
	"os"
	"path"
)

func IsFolder(name string) bool {
	info, _ := os.Stat(name)
	if info != nil {
		return info.IsDir()
	}
	return false
}

func Exists(name string) bool {
	i, err := os.Stat(name)
	if err == nil {
		return true
	}
	if os.IsNotExist(err){
		return false
	}
	return i.IsDir()
}

func IsEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1) // Or f.Readdir(1)
	if err == io.EOF {
		return true, nil
	}
	return false, err // Either not empty or error, suits both cases
}

func CreateParentFolders(file string) error {
	dir := path.Dir(file)
	if !Exists(dir) {
		return os.MkdirAll(dir, os.ModePerm)
	}
	return nil
}
