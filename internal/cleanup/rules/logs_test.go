package rules

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
)

// sparseFile creates a file that reports size bytes without using that much disk.
func sparseFile(t *testing.T, path string, size int64) {
	t.Helper()
	touch(t, path, 0, longAgo)
	if err := os.Truncate(path, size); err != nil {
		t.Fatal(err)
	}
}

func TestBigLogsRuleFindsLargeLogsAndHeapDumps(t *testing.T) {
	env := testEnv(t)
	sparseFile(t, env.InHome("projects/app/metro.log"), bigLogMinSize)
	sparseFile(t, env.InHome("projects/app/server.log.1"), bigLogMinSize)
	sparseFile(t, env.InHome("java_pid123.hprof"), bigLogMinSize)
	sparseFile(t, env.InHome("projects/app/small.log"), bigLogMinSize-1)
	sparseFile(t, env.InHome("projects/app/video.mov"), bigLogMinSize)
	sparseFile(t, env.InHome("projects/app/node_modules/pkg/debug.log"), bigLogMinSize)
	sparseFile(t, env.InHome("projects/app/.git/huge.log"), bigLogMinSize)
	sparseFile(t, env.InHome("Library/Logs/App/claimed.log"), bigLogMinSize)
	sparseFile(t, env.InHome("Library/Mobile Documents/cloud.log"), bigLogMinSize)
	sparseFile(t, env.InHome("Library/Application Support/Tool/debug.log"), bigLogMinSize)
	sparseFile(t, env.InHome(".npm/_cacache/huge.log"), bigLogMinSize)

	items, err := bigLogsRule{}.Scan(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	if got, want := titles(items), []string{"debug.log", "java_pid123.hprof", "metro.log", "server.log.1"}; !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	for _, item := range items {
		if item.Safety != cleanup.SafetyReview || item.Group != bigLogsGroup || item.LastUsed.IsZero() {
			t.Errorf("%s: safety=%v group=%q lastUsed=%v", item.Title, item.Safety, item.Group, item.LastUsed)
		}
	}
}

func TestIsLogOrDump(t *testing.T) {
	for name, want := range map[string]bool{
		"app.log":           true,
		"APP.LOG":           true,
		"app.log.1":         true,
		"app.log.gz":        true,
		"npm-debug.log.123": true,
		"java_pid1.hprof":   true,
		"catalog.json":      false,
		"blog.md":           false,
		"session.jsonl":     false,
	} {
		if got := isLogOrDump(name); got != want {
			t.Errorf("isLogOrDump(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestAppLogsAreGroupedAndNamedByOwner(t *testing.T) {
	env := testEnv(t)
	touch(t, env.InHome("Library/Application Support/Cursor/logs/main.log"), 1, longAgo)
	touch(t, env.InHome("Library/Containers/com.docker.docker/Data/log/vm.log"), 1, longAgo)
	touch(t, env.InHome("Library/Logs/DiagnosticReports/crash.ips"), 1, longAgo)

	var items []cleanup.Item
	for _, rule := range appLogRules() {
		if _, isWalk := rule.(bigLogsRule); isWalk {
			continue
		}
		found, err := rule.Scan(context.Background(), env)
		if err != nil {
			t.Fatal(err)
		}
		items = append(items, found...)
	}

	want := []string{"App logs: Cursor/logs", "App logs: com.docker.docker/Data/log", "Logs: DiagnosticReports"}
	if got := titles(items); !slices.Equal(got, want) {
		t.Fatalf("items = %v, want %v", got, want)
	}
	for _, item := range items {
		if item.Group != logsGroup || item.Safety != cleanup.SafetySafe {
			t.Errorf("%s: group=%q safety=%v, want safe in %q", item.Title, item.Group, item.Safety, logsGroup)
		}
	}
}

func TestElectronPartitionCachesAreFound(t *testing.T) {
	env := testEnv(t)
	touch(t, env.InHome("Library/Application Support/Notion/Partitions/notion/Service Worker/CacheStorage/blob"), 1, longAgo)
	touch(t, env.InHome("Library/Application Support/Notion/Partitions/notion/IndexedDB/keep"), 1, longAgo)

	rule := safe(caches, "Notion app cache", appCachePatterns("Library/Application Support/Notion")...)
	items, err := rule.Scan(context.Background(), env)
	if err != nil {
		t.Fatal(err)
	}

	want := env.InHome("Library/Application Support/Notion/Partitions/notion/Service Worker/CacheStorage")
	if len(items) != 1 || !slices.Equal(items[0].Paths, []string{want}) {
		t.Errorf("items = %+v, want only the partition's CacheStorage", items)
	}
}

func TestMatchLabel(t *testing.T) {
	env := testEnv(t)
	for path, want := range map[string]string{
		env.InHome("Library/Caches/Homebrew"):                 "Homebrew",
		env.InHome("Library/Application Support/Cursor/logs"): "Cursor/logs",
		env.InHome("Library/Containers/com.x/Data/log"):       "com.x/Data/log",
		filepath.Join(env.Home, "logs"):                       "logs",
	} {
		if got := matchLabel(env, path); got != want {
			t.Errorf("matchLabel(%q) = %q, want %q", path, got, want)
		}
	}
}
