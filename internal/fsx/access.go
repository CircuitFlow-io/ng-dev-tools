package fsx

import (
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"
)

// NeedsRoot reports whether deleting path likely requires elevated privileges:
// it is owned by another user or its parent directory is not writable by us.
func NeedsRoot(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != os.Geteuid() {
		return true
	}
	return unix.Access(filepath.Dir(path), unix.W_OK) != nil
}
