package rules

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
)

const (
	files = cleanup.CategoryFiles
	// minInstallerSize ignores tiny archives that are not worth listing.
	minInstallerSize = 1 << 20
)

var largeFileDirs = []string{"Downloads", "Desktop"}

var installerExtensions = map[string]bool{
	".dmg": true, ".pkg": true, ".mpkg": true, ".zip": true, ".xip": true, ".iso": true,
	".crdownload": true, ".part": true,
}

// packageExtensions are macOS bundles that look like folders but should be treated as a unit.
var packageExtensions = map[string]bool{
	".app": true, ".photoslibrary": true, ".fcpbundle": true, ".logicx": true, ".band": true,
}

// largeFilesRule finds big files and old installers that have not been opened recently.
type largeFilesRule struct{}

func (largeFilesRule) Name() string               { return "Large old files" }
func (largeFilesRule) Category() cleanup.Category { return files }

func (largeFilesRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	var items []cleanup.Item
	for _, dir := range largeFileDirs {
		err := filepath.WalkDir(env.InHome(dir), func(path string, d fs.DirEntry, err error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err != nil {
				return nil
			}
			if d.IsDir() {
				return skipHiddenOrPackage(d)
			}
			if item, ok := largeFileItem(env, path, d); ok {
				items = append(items, item)
			}
			return nil
		})
		if err != nil {
			return items, err
		}
	}
	return items, nil
}

func skipHiddenOrPackage(d fs.DirEntry) error {
	name := d.Name()
	if strings.HasPrefix(name, ".") || packageExtensions[strings.ToLower(filepath.Ext(name))] {
		return fs.SkipDir
	}
	return nil
}

func largeFileItem(env cleanup.Env, path string, d fs.DirEntry) (cleanup.Item, bool) {
	if !d.Type().IsRegular() {
		return cleanup.Item{}, false
	}
	info, err := d.Info()
	if err != nil {
		return cleanup.Item{}, false
	}
	isInstaller := installerExtensions[strings.ToLower(filepath.Ext(path))] && info.Size() >= minInstallerSize
	isLarge := info.Size() >= env.LargeFileMinSize
	worthListing := isInstaller || isLarge
	touched := fsx.LastTouched(info)
	if !worthListing || !env.IsStale(touched) {
		return cleanup.Item{}, false
	}
	item := cleanup.PathItem(files, cleanup.SafetyReview, filepath.Base(path), path)
	item.LastUsed = touched
	return item, true
}
