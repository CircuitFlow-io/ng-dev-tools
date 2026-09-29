package projects

import (
	"context"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
)

// maxWalkedEntries bounds the walk of a large project; the newest change is nearly always found long before.
const maxWalkedEntries = 20000

// generatedDirs hold dependencies and build output, which change without the project being worked on.
var generatedDirs = map[string]bool{
	"node_modules": true,
	"Pods":         true,
	"build":        true,
	"dist":         true,
	"DerivedData":  true,
	"coverage":     true,
	"vendor":       true,
	"target":       true,
}

// lastChanged is the newest modification time among the project's own files, or its last commit or checkout.
func lastChanged(ctx context.Context, dir string) time.Time {
	newest := gitActivity(dir)
	walked := 0
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || ctx.Err() != nil {
			return skipOnError(ctx)
		}
		walked++
		if walked > maxWalkedEntries {
			return filepath.SkipAll
		}
		if entry.IsDir() && path != dir && (isHidden(entry.Name()) || generatedDirs[entry.Name()]) {
			return filepath.SkipDir
		}
		if info, err := entry.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest
}

func skipOnError(ctx context.Context) error {
	if ctx.Err() != nil {
		return filepath.SkipAll
	}
	return nil
}

// gitActivity is the time of the last commit, checkout or pull, from the HEAD reflog.
func gitActivity(dir string) time.Time {
	git := GitDir(dir)
	if git == "" {
		return time.Time{}
	}
	return fsx.ModTime(filepath.Join(git, "logs", "HEAD"))
}
