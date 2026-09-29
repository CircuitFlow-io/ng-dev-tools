// Package fsx holds filesystem helpers that the standard library does not provide.
package fsx

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

const (
	statBlockSize = 512
	// reportEvery is how many entries DiskUsageWithProgress walks between progress callbacks.
	reportEvery = 1024
)

type inode struct {
	dev int32
	ino uint64
}

// DiskUsage returns the bytes allocated on disk for path and everything below it, like `du`.
// Symlinks are never followed, hard links are counted once, and unreadable entries are skipped.
func DiskUsage(ctx context.Context, path string) int64 {
	return DiskUsageWithProgress(ctx, path, nil)
}

// DiskUsageWithProgress is DiskUsage that periodically reports bytes counted since the last
// report, so callers can show a running total for large trees. onProgress may be nil.
func DiskUsageWithProgress(ctx context.Context, path string, onProgress func(delta int64)) int64 {
	seen := make(map[inode]bool)
	var total, unreported int64
	var entries int
	_ = filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return skipUnreadable(d)
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		size := allocatedBytes(info, seen)
		total += size
		unreported += size
		if entries++; entries%reportEvery == 0 && onProgress != nil {
			onProgress(unreported)
			unreported = 0
		}
		return nil
	})
	if onProgress != nil && unreported > 0 {
		onProgress(unreported)
	}
	return total
}

func skipUnreadable(d fs.DirEntry) error {
	if d != nil && d.IsDir() {
		return fs.SkipDir
	}
	return nil
}

func allocatedBytes(info fs.FileInfo, seen map[inode]bool) int64 {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return info.Size()
	}
	if stat.Nlink > 1 && !info.IsDir() {
		key := inode{dev: stat.Dev, ino: stat.Ino}
		if seen[key] {
			return 0
		}
		seen[key] = true
	}
	return stat.Blocks * statBlockSize
}

// Exists reports whether path exists without following a final symlink.
func Exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
