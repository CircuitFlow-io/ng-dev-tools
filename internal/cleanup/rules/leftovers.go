package rules

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

// minIndexedApps is how many apps Spotlight must report before we trust it to know what is
// installed. Fewer usually means indexing is off, and everything would look orphaned.
const minIndexedApps = 10

var bundleIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9_-]+){2,}$`)

// supportLocation is a ~/Library folder whose entries are named after bundle IDs.
type supportLocation struct {
	dir    string
	suffix string
}

var supportLocations = []supportLocation{
	{"Library/Application Support", ""},
	{"Library/Caches", ""},
	{"Library/Preferences", ".plist"},
	{"Library/Saved Application State", ".savedState"},
	{"Library/HTTPStorages", ""},
	{"Library/HTTPStorages", ".binarycookies"},
	{"Library/WebKit", ""},
}

// leftoversRule finds support files of applications that are no longer installed.
type leftoversRule struct{}

func (leftoversRule) Name() string               { return "Leftovers from uninstalled apps" }
func (leftoversRule) Category() cleanup.Category { return apps }

func (leftoversRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	installed, err := installedBundleIDs(ctx, env)
	if err != nil {
		return nil, err
	}
	orphans := make(map[string][]string)
	var order []string
	for _, loc := range supportLocations {
		for id, path := range bundleEntries(env.InHome(loc.dir), loc.suffix) {
			if installed.covers(id) {
				continue
			}
			if _, seen := orphans[id]; !seen {
				order = append(order, id)
			}
			orphans[id] = append(orphans[id], path)
		}
	}

	items := make([]cleanup.Item, 0, len(order))
	for _, id := range order {
		item := cleanup.PathItem(apps, cleanup.SafetyReview, "Leftovers: "+id, orphans[id]...)
		item.Detail = fmt.Sprintf("%d location(s) from an app that is no longer installed", len(orphans[id]))
		items = append(items, item)
	}
	return items, nil
}

// bundleEntries maps bundle IDs to the entries of dir named after them.
func bundleEntries(dir, suffix string) map[string]string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	found := make(map[string]string)
	for _, entry := range entries {
		name := entry.Name()
		if suffix != "" && !strings.HasSuffix(name, suffix) {
			continue
		}
		id := strings.TrimSuffix(name, suffix)
		if bundleIDPattern.MatchString(id) && !strings.HasPrefix(strings.ToLower(id), appleVendor) {
			found[id] = filepath.Join(dir, name)
		}
	}
	return found
}

func installedBundleIDs(ctx context.Context, env cleanup.Env) (bundleIDSet, error) {
	out, err := env.Runner.Run(ctx, "mdfind", "-0", "kMDItemContentType == 'com.apple.application-bundle'")
	if err != nil {
		return nil, fmt.Errorf("listing installed apps: %w", err)
	}
	paths := strings.Split(strings.TrimRight(string(out), "\x00"), "\x00")
	if len(paths) < minIndexedApps {
		return nil, errors.New("spotlight index looks incomplete; skipped leftover detection")
	}
	paths = append(paths, installedApps(env)...)

	ids := make(bundleIDSet)
	for _, path := range paths {
		if id, err := macos.BundleID(path); err == nil && id != "" {
			ids.add(id)
		}
	}
	return ids, nil
}

// bundleIDSet answers "does something installed own this ID?" conservatively: a shared vendor
// prefix (com.google.*) counts as installed, because helpers and agents rarely match exactly.
type bundleIDSet map[string]bool

const vendorComponents = 2

func (s bundleIDSet) add(id string) {
	id = strings.ToLower(id)
	s[id] = true
	s[vendorOf(id)] = true
}

func (s bundleIDSet) covers(id string) bool {
	id = strings.ToLower(id)
	return s[id] || s[vendorOf(id)]
}

func vendorOf(id string) string {
	parts := strings.SplitN(id, ".", vendorComponents+1)
	return strings.Join(parts[:min(len(parts), vendorComponents)], ".") + ".*"
}
