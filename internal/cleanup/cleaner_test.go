package cleanup

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nasserghiasi/ng-dev-tools/internal/fsx"
	"github.com/nasserghiasi/ng-dev-tools/internal/macos/macostest"
)

func newTestCleaner(home string, runner *macostest.Runner) Cleaner {
	return Cleaner{Guard: NewGuard(home), Runner: runner}
}

func TestCleanRemovesPathsAndReportsProgress(t *testing.T) {
	home := t.TempDir()
	cache := makeDir(t, filepath.Join(home, "Library", "Caches", "app"), 10)
	item := PathItem(CategoryCaches, SafetySafe, "app", cache)
	item.Size = 100

	var last CleanProgress
	results := newTestCleaner(home, &macostest.Runner{}).Clean(context.Background(), []Item{item}, func(p CleanProgress) { last = p })

	if results[0].Err != nil {
		t.Fatalf("Clean: %v", results[0].Err)
	}
	if fsx.Exists(cache) {
		t.Error("cache still exists")
	}
	if last.Done != 1 || last.BytesDone != 100 || last.BytesTotal != 100 {
		t.Errorf("progress = %+v", last)
	}
}

func TestCleanDryRunKeepsFilesAndLogs(t *testing.T) {
	home := t.TempDir()
	cache := makeDir(t, filepath.Join(home, "cache"), 10)
	var log bytes.Buffer
	cleaner := newTestCleaner(home, &macostest.Runner{})
	cleaner.DryRun, cleaner.Log = true, &log

	results := cleaner.Clean(context.Background(), []Item{PathItem(CategoryCaches, SafetySafe, "cache", cache)}, nil)

	if results[0].Err != nil || !fsx.Exists(cache) {
		t.Errorf("dry run touched the file or failed: %v", results[0].Err)
	}
	if !bytes.Contains(log.Bytes(), []byte("dry-run")) {
		t.Errorf("log = %q, want a dry-run entry", log.String())
	}
}

func TestCleanRefusesGuardedPaths(t *testing.T) {
	home := t.TempDir()
	documents := makeDir(t, filepath.Join(home, "Documents"), 10)

	results := newTestCleaner(home, &macostest.Runner{}).Clean(context.Background(), []Item{PathItem(CategoryFiles, SafetyReview, "docs", documents)}, nil)

	if results[0].Err == nil || !fsx.Exists(documents) {
		t.Error("guard did not stop deleting ~/Documents")
	}
}

func TestCleanRunsRemoveCommand(t *testing.T) {
	runner := &macostest.Runner{}
	item := Item{Title: "sim", RemoveCommand: []string{"xcrun", "simctl", "delete", "U1"}}

	results := newTestCleaner(t.TempDir(), runner).Clean(context.Background(), []Item{item}, nil)

	if results[0].Err != nil || !slices.Equal(runner.Calls(), []string{"xcrun simctl delete U1"}) {
		t.Errorf("calls = %v, err = %v", runner.Calls(), results[0].Err)
	}
}

func TestCleanRootItems(t *testing.T) {
	home := t.TempDir()
	item := PathItem(CategoryCaches, SafetyReview, "system", "/Library/Caches/com.example")
	item.NeedsRoot = true

	t.Run("without authorization", func(t *testing.T) {
		results := newTestCleaner(home, &macostest.Runner{}).Clean(context.Background(), []Item{item}, nil)
		if !errors.Is(results[0].Err, ErrRootNotAuthorized) {
			t.Errorf("err = %v, want ErrRootNotAuthorized", results[0].Err)
		}
	})

	t.Run("with authorization", func(t *testing.T) {
		runner := &macostest.Runner{}
		cleaner := newTestCleaner(home, runner)
		cleaner.RootAuthorized = true
		cleaner.Clean(context.Background(), []Item{item}, nil)
		if want := []string{"sudo -n rm -rf -- /Library/Caches/com.example"}; !slices.Equal(runner.Calls(), want) {
			t.Errorf("calls = %v, want %v", runner.Calls(), want)
		}
	})
}

func TestCleanStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	results := newTestCleaner(t.TempDir(), &macostest.Runner{}).Clean(ctx, []Item{{Title: "a", RemoveCommand: []string{"true"}}}, nil)

	if len(results) != 0 {
		t.Errorf("results = %v, want none after cancellation", results)
	}
}
