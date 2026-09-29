package rules

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/fsx"
	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
)

const (
	apps        = cleanup.CategoryApps
	appleVendor = "com.apple."
	appBundle   = ".app"
)

// unusedAppsRule finds applications not opened within the staleness window, together with the
// support files they left in ~/Library.
type unusedAppsRule struct{}

func (unusedAppsRule) Name() string               { return "Unused applications" }
func (unusedAppsRule) Category() cleanup.Category { return apps }

func (unusedAppsRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	bundles := installedApps(env)
	lastUsed, err := macos.LastUsedDates(ctx, env.Runner, bundles)
	if err != nil {
		return nil, err
	}
	running, err := macos.RunningExecutables(ctx, env.Runner)
	if err != nil {
		return nil, err
	}

	var items []cleanup.Item
	for _, bundle := range bundles {
		if item, ok := unusedAppItem(env, bundle, lastUsed, running); ok {
			items = append(items, item)
		}
	}
	return items, nil
}

func installedApps(env cleanup.Env) []string {
	var bundles []string
	for _, dir := range []string{"/Applications", env.InHome("Applications")} {
		top, _ := filepath.Glob(filepath.Join(dir, "*"+appBundle))
		nested, _ := filepath.Glob(filepath.Join(dir, "*", "*"+appBundle))
		bundles = append(bundles, top...)
		bundles = append(bundles, nested...)
	}
	return bundles
}

func unusedAppItem(env cleanup.Env, bundle string, lastUsed map[string]time.Time, running []string) (cleanup.Item, bool) {
	id, err := macos.BundleID(bundle)
	if err != nil || id == "" || strings.HasPrefix(id, appleVendor) || isRunning(bundle, running) {
		return cleanup.Item{}, false
	}
	used, ok := lastUsed[bundle]
	if !ok {
		used = fsx.ModTime(bundle)
	}
	if !env.IsStale(used) {
		return cleanup.Item{}, false
	}

	name := strings.TrimSuffix(filepath.Base(bundle), appBundle)
	paths := append([]string{bundle}, appSupportFiles(env, id, name)...)
	item := cleanup.PathItem(apps, cleanup.SafetyReview, name, paths...)
	item.LastUsed = used
	if len(paths) > 1 {
		item.Detail = bundle + " + support files"
	}
	return item, true
}

func isRunning(bundle string, running []string) bool {
	prefix := bundle + "/"
	return slices.ContainsFunc(running, func(exe string) bool { return strings.HasPrefix(exe, prefix) })
}

// minAppNameMatch avoids matching support folders by very short, generic app names.
const minAppNameMatch = 4

func appSupportFiles(env cleanup.Env, bundleID, appName string) []string {
	candidates := []string{
		"Library/Application Support/" + bundleID,
		"Library/Caches/" + bundleID,
		"Library/Preferences/" + bundleID + ".plist",
		"Library/Saved Application State/" + bundleID + ".savedState",
		"Library/HTTPStorages/" + bundleID,
		"Library/HTTPStorages/" + bundleID + ".binarycookies",
		"Library/WebKit/" + bundleID,
		"Library/Cookies/" + bundleID + ".binarycookies",
	}
	if len(appName) >= minAppNameMatch {
		candidates = append(candidates,
			"Library/Application Support/"+appName,
			"Library/Caches/"+appName,
			"Library/Logs/"+appName,
		)
	}

	var found []string
	for _, rel := range candidates {
		if path := env.InHome(rel); fsx.Exists(path) {
			found = append(found, path)
		}
	}
	return found
}
