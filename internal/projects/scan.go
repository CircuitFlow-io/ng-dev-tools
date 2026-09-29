package projects

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
)

const maxParallelScans = 8

var xcodeBundleExtensions = []string{".xcodeproj", ".xcworkspace"}

// Scan lists the projects in root, most recent activity first. opened holds when each project
// path was last opened with ngt.
func Scan(ctx context.Context, root string, opened map[string]time.Time) ([]Project, error) {
	dirs, err := Dirs(root)
	if err != nil {
		return nil, err
	}

	projects := make([]Project, len(dirs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelScans)
	for i, dir := range dirs {
		g.Go(func() error {
			projects[i] = inspect(gctx, root, dir, opened[dir])
			return nil
		})
	}
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(projects, func(a, b Project) int {
		return cmp.Or(b.LastActivity().Compare(a.LastActivity()), cmp.Compare(a.Name, b.Name))
	})
	return projects, nil
}

// Dirs returns root's folders, replacing a folder that only groups other folders with
// those folders, and leaving out empty ones.
func Dirs(root string) ([]string, error) {
	children, err := visibleSubdirs(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, dir := range children {
		if isProject(dir) {
			dirs = append(dirs, dir)
			continue
		}
		grouped, _ := visibleSubdirs(dir)
		for _, sub := range grouped {
			if hasVisibleEntries(sub) {
				dirs = append(dirs, sub)
			}
		}
	}
	return dirs, nil
}

func inspect(ctx context.Context, root, dir string, opened time.Time) Project {
	name, err := filepath.Rel(root, dir)
	if err != nil {
		name = filepath.Base(dir)
	}
	return Project{
		Name:    name,
		Path:    dir,
		Git:     GitDir(dir) != "",
		Branch:  Branch(dir),
		Opened:  opened,
		Changed: lastChanged(ctx, dir),
	}
}

// isProject reports whether dir is a codebase rather than a folder grouping several: it is a git
// repository, has files of its own, or holds an Xcode project.
func isProject(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		name := entry.Name()
		switch {
		case name == ".git":
			return true
		case isHidden(name):
			continue
		case !entry.IsDir(), slices.Contains(xcodeBundleExtensions, filepath.Ext(name)):
			return true
		}
	}
	return false
}

func visibleSubdirs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, entry := range entries {
		if entry.IsDir() && !isHidden(entry.Name()) {
			dirs = append(dirs, filepath.Join(dir, entry.Name()))
		}
	}
	return dirs, nil
}

func hasVisibleEntries(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	return slices.ContainsFunc(entries, func(e os.DirEntry) bool { return !isHidden(e.Name()) })
}

func isHidden(name string) bool {
	return strings.HasPrefix(name, ".")
}
