package fsx

import (
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// NewestModTime returns the latest modification time among dir itself and its direct
// entries, ignoring entries whose names are in skip.
func NewestModTime(dir string, skip map[string]bool) time.Time {
	newest := ModTime(dir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return newest
	}
	for _, entry := range entries {
		if skip[entry.Name()] {
			continue
		}
		if t := ModTime(filepath.Join(dir, entry.Name())); t.After(newest) {
			newest = t
		}
	}
	return newest
}

// LastTouched returns the later of a file's modification and access times.
func LastTouched(info os.FileInfo) time.Time {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.ModTime()
	}
	accessed := time.Unix(stat.Atimespec.Unix())
	if accessed.After(info.ModTime()) {
		return accessed
	}
	return info.ModTime()
}

// ModTime returns path's modification time without following a final symlink, or zero if it is missing.
func ModTime(path string) time.Time {
	info, err := os.Lstat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
