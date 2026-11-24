package rules

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	archivesPattern = "Library/Developer/Xcode/Archives/*/*.xcarchive"
	archivesGroup   = "Xcode archives"
)

// archivesRule lists every Xcode archive, whatever its age, so any of them can be picked.
// They are never preselected: an archive holds the dSYMs needed to symbolicate crash reports.
type archivesRule struct{}

func (archivesRule) Name() string               { return "Xcode archives" }
func (archivesRule) Category() cleanup.Category { return dev }

func (archivesRule) Scan(_ context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	paths, err := filepath.Glob(env.InHome(archivesPattern))
	if err != nil {
		return nil, err
	}
	items := make([]cleanup.Item, 0, len(paths))
	for _, path := range paths {
		items = append(items, archiveItem(path))
	}
	return items, nil
}

func archiveItem(path string) cleanup.Item {
	info, err := macos.ReadArchive(path)
	created := info.CreationDate
	if created.IsZero() {
		created = fsx.ModTime(path)
	}
	item := cleanup.PathItem(dev, cleanup.SafetyReview, "Xcode archive: "+archiveLabel(path, info, err), path)
	item.LastUsed = created
	item.Group = archivesGroup
	return item
}

// archiveLabel reads like "Treddle 0.1.0 (52)", falling back to the bundle's file name.
func archiveLabel(path string, info macos.ArchiveInfo, readErr error) string {
	if readErr != nil || info.Name == "" {
		return filepath.Base(path)
	}
	if info.App.Version == "" {
		return info.Name
	}
	if info.App.Build == "" {
		return fmt.Sprintf("%s %s", info.Name, info.App.Version)
	}
	return fmt.Sprintf("%s %s (%s)", info.Name, info.App.Version, info.App.Build)
}
