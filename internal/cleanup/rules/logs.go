package rules

import (
	"context"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
)

const (
	logsGroup    = "Logs"
	bigLogsGroup = "Big log files & heap dumps"
	// bigLogMinSize is where a single log file stops being noise and starts costing space.
	bigLogMinSize = 20 << 20
)

// appLogRules cover log folders apps keep outside ~/Library/Logs. Sandboxed app containers are
// left out: macOS checks access to each one, which is slow and can prompt the user.
func appLogRules() []cleanup.Rule {
	return []cleanup.Rule{
		grouped(logsGroup, excluding(separate(safe(caches, "Logs", "Library/Logs/*")), "ngt")),
		grouped(logsGroup, separate(safe(caches, "App logs",
			"Library/Application Support/*/logs",
			"Library/Application Support/*/Logs",
			"Library/Application Support/*/*/Logs",
			"Library/Containers/com.docker.docker/Data/log",
			".local/share/*/log",
			".local/state/*/log",
			"Library/Preferences/.wrangler/logs",
		))),
		bigLogsRule{},
	}
}

// bigLogRoots are where stray logs turn up. The rest of ~/Library is left out: it is huge, the
// parts with logs (~/Library/Logs, Xcode) are covered by other rules, and sandboxed app
// containers are slow to enter and can prompt the user.
var bigLogRoots = []string{"", "Library/Application Support"}

// bigLogSkipDirs are home-relative folders the walk never enters: walked as their own root,
// owned by other rules, package stores with many files and no logs, or personal data.
var bigLogSkipDirs = []string{
	"Library",
	".Trash",
	".npm", ".nvm", ".cache", ".gradle", ".m2", ".cargo", ".rustup", ".pub-cache", ".cocoapods", ".bun",
	"go/pkg",
	"Library/Application Support/MobileSync",
}

// bigLogSkipNames are folder names skipped wherever they appear.
var bigLogSkipNames = map[string]bool{
	".git": true, "node_modules": true, "Pods": true, "DerivedData": true, ".venv": true, "venv": true,
}

// bigLogsRule finds single large log files and Java heap dumps, such as a runaway dev-server log
// in a project or an Android Studio .hprof left in the home folder.
type bigLogsRule struct{}

func (bigLogsRule) Name() string               { return "Big log files" }
func (bigLogsRule) Category() cleanup.Category { return caches }

func (bigLogsRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	skip := make(map[string]bool, len(bigLogSkipDirs))
	for _, dir := range bigLogSkipDirs {
		skip[env.InHome(dir)] = true
	}
	found := make([][]cleanup.Item, len(bigLogRoots))
	var g errgroup.Group
	for i, root := range bigLogRoots {
		g.Go(func() error {
			var err error
			found[i], err = walkForBigLogs(ctx, env.InHome(root), skip)
			return err
		})
	}
	err := g.Wait()
	return slices.Concat(found...), err
}

func walkForBigLogs(ctx context.Context, root string, skip map[string]bool) ([]cleanup.Item, error) {
	var items []cleanup.Item
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			return skipBigLogDir(path, d, skip)
		}
		if item, ok := bigLogItem(path, d); ok {
			items = append(items, item)
		}
		return nil
	})
	return items, err
}

func skipBigLogDir(path string, d fs.DirEntry, skip map[string]bool) error {
	name := d.Name()
	if skip[path] || bigLogSkipNames[name] || packageExtensions[strings.ToLower(filepath.Ext(name))] {
		return fs.SkipDir
	}
	return nil
}

func bigLogItem(path string, d fs.DirEntry) (cleanup.Item, bool) {
	if !d.Type().IsRegular() || !isLogOrDump(d.Name()) {
		return cleanup.Item{}, false
	}
	info, err := d.Info()
	if err != nil || info.Size() < bigLogMinSize {
		return cleanup.Item{}, false
	}
	item := cleanup.PathItem(caches, cleanup.SafetyReview, d.Name(), path)
	item.LastUsed = fsx.LastTouched(info)
	item.Group = bigLogsGroup
	return item, true
}

// isLogOrDump matches "app.log", rotated logs such as "app.log.1" or "app.log.gz",
// "npm-debug.log.123" and Java heap dumps.
func isLogOrDump(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".log") || strings.Contains(lower, ".log.") || strings.HasSuffix(lower, ".hprof")
}
