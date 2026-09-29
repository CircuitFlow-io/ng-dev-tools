package rules

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
)

const caches = cleanup.CategoryCaches

// appleCachePattern matches caches of macOS services. They are rebuilt too, but a user may not
// expect system caches to be preselected, so they are listed for review.
const appleCachePattern = "com.apple.*"

// cacheExclusions are ~/Library/Caches entries owned by system services that are syncing or
// downloading; removing them mid-flight can lose work.
var cacheExclusions = []string{
	"CloudKit", "com.apple.bird", "com.apple.nsurlsessiond", "com.apple.HomeKit",
	"com.apple.containermanagerd", "FamilyCircle", "com.apple.ap.adprivacyd",
}

var chromiumCacheDirs = []string{
	"Cache", "Code Cache", "GPUCache", "DawnCache", "DawnGraphiteCache", "DawnWebGPUCache",
	"Service Worker/CacheStorage", "Service Worker/ScriptCache",
}

type namedRoot struct{ name, root string }

var chromiumBrowsers = []namedRoot{
	{"Chrome", "Library/Application Support/Google/Chrome"},
	{"Brave", "Library/Application Support/BraveSoftware/Brave-Browser"},
	{"Edge", "Library/Application Support/Microsoft Edge"},
	{"Arc", "Library/Application Support/Arc/User Data"},
	{"Vivaldi", "Library/Application Support/Vivaldi"},
	{"Chromium", "Library/Application Support/Chromium"},
}

var electronCacheDirs = append([]string{"CachedData", "CachedExtensionVSIXs"}, chromiumCacheDirs...)

var electronApps = []namedRoot{
	{"Slack", "Library/Application Support/Slack"},
	{"Discord", "Library/Application Support/discord"},
	{"VS Code", "Library/Application Support/Code"},
	{"Cursor", "Library/Application Support/Cursor"},
	{"Notion", "Library/Application Support/Notion"},
	{"Figma", "Library/Application Support/Figma"},
	{"Postman", "Library/Application Support/Postman"},
	{"Obsidian", "Library/Application Support/obsidian"},
	{"Signal", "Library/Application Support/Signal"},
	{"Claude", "Library/Application Support/Claude"},
	{"Teams", "Library/Application Support/Microsoft/Teams"},
}

func cacheRules() []cleanup.Rule {
	rules := []cleanup.Rule{
		safe(caches, "Spotify offline cache", "Library/Application Support/Spotify/PersistentCache"),
		safe(caches, "iOS software updates", "Library/iTunes/iPhone Software Updates", "Library/iTunes/iPad Software Updates"),
		review(caches, "Mail attachments cache", "Library/Containers/com.apple.mail/Data/Library/Mail Downloads"),
		separate(review(caches, "iOS device backup", "Library/Application Support/MobileSync/Backup/*")),
	}
	for _, b := range chromiumBrowsers {
		rules = append(rules, safe(caches, b.name+" browser cache", profileCachePatterns(b.root)...))
	}
	for _, app := range electronApps {
		rules = append(rules, safe(caches, app.name+" app cache", appCachePatterns(app.root)...))
	}
	return rules
}

// catchAllCacheRules sweep whole cache folders. They run after the specific rules so an exact
// path match keeps its more descriptive title.
func catchAllCacheRules() []cleanup.Rule {
	return []cleanup.Rule{
		excluding(separate(safe(caches, "App cache", "Library/Caches/*")), append(cacheExclusions, appleCachePattern)...),
		excluding(review(caches, "Apple service caches", "Library/Caches/"+appleCachePattern), cacheExclusions...),
		excluding(separate(safe(caches, "Logs", "Library/Logs/*")), "ngt"),
		separate(safe(caches, "Cache", ".cache/*")),
		separate(review(caches, "System cache", "/Library/Caches/*")),
		separate(review(caches, "System logs", "/Library/Logs/*")),
		trashRule{},
	}
}

func profileCachePatterns(root string) []string {
	var patterns []string
	for _, dir := range chromiumCacheDirs {
		patterns = append(patterns, filepath.Join(root, "*", dir))
	}
	return patterns
}

func appCachePatterns(root string) []string {
	patterns := make([]string, 0, len(electronCacheDirs))
	for _, dir := range electronCacheDirs {
		patterns = append(patterns, filepath.Join(root, dir))
	}
	return patterns
}

// trashRule empties the Trash by deleting its contents, never the folder itself.
type trashRule struct{}

func (trashRule) Name() string               { return "Trash" }
func (trashRule) Category() cleanup.Category { return caches }

func (trashRule) Scan(_ context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	trash := env.InHome(".Trash")
	entries, err := os.ReadDir(trash)
	if errors.Is(err, fs.ErrPermission) {
		return nil, fmt.Errorf("cannot read the Trash; grant Full Disk Access to your terminal to include it")
	}
	if err != nil || len(entries) == 0 {
		return nil, nil
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		paths = append(paths, filepath.Join(trash, entry.Name()))
	}
	item := cleanup.PathItem(caches, cleanup.SafetyReview, "Trash", paths...)
	item.Detail = fmt.Sprintf("%s (%d items)", trash, len(entries))
	return []cleanup.Item{item}, nil
}
