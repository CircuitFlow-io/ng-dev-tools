package fsx

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const ownerWritable = 0o200

// RemoveAll deletes path like os.RemoveAll. If that fails on permissions, it makes our own
// read-only directories writable (as Go's module cache and some package stores are) and retries.
func RemoveAll(path string) error {
	err := os.RemoveAll(path)
	if err == nil || !errors.Is(err, fs.ErrPermission) {
		return err
	}
	makeDirsWritable(path)
	return os.RemoveAll(path)
}

func makeDirsWritable(root string) {
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Mode().Perm()&ownerWritable != 0 {
			return nil
		}
		_ = os.Chmod(path, info.Mode().Perm()|ownerWritable)
		return nil
	})
}
