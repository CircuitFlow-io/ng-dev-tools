package cleanup

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type fakeRule struct {
	name     string
	category Category
	items    []Item
	err      error
}

func (r fakeRule) Name() string                              { return r.name }
func (r fakeRule) Category() Category                        { return r.category }
func (r fakeRule) Scan(context.Context, Env) ([]Item, error) { return r.items, r.err }

func makeDir(t *testing.T, path string, bytes int) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "data"), make([]byte, bytes), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestScanMeasuresSortsAndFilters(t *testing.T) {
	home := t.TempDir()
	small := makeDir(t, filepath.Join(home, "small"), 1024)
	medium := makeDir(t, filepath.Join(home, "medium"), 512*1024)
	large := makeDir(t, filepath.Join(home, "large"), 2*1024*1024)
	scanner := Scanner{Rules: []Rule{
		fakeRule{name: "files", category: CategoryFiles, items: []Item{PathItem(CategoryFiles, SafetyReview, "large", large)}},
		fakeRule{name: "caches", category: CategoryCaches, items: []Item{
			PathItem(CategoryCaches, SafetySafe, "small", small),
			PathItem(CategoryCaches, SafetySafe, "medium", medium),
		}},
		fakeRule{name: "broken", category: CategoryDeveloper, err: errors.New("boom")},
	}}

	var updates int
	result := scanner.Scan(context.Background(), func(ScanProgress) { updates++ })

	if len(result.Items) != 2 || result.Items[0].Title != "medium" || result.Items[1].Title != "large" {
		t.Fatalf("items = %+v, want medium then large (grouped by category, tiny items dropped)", result.Items)
	}
	if result.Items[1].Size < 2*1024*1024 {
		t.Errorf("large size = %d, want >= 2 MiB", result.Items[1].Size)
	}
	if len(result.Warnings) != 1 {
		t.Errorf("warnings = %v, want the broken rule", result.Warnings)
	}
	if updates == 0 {
		t.Error("no progress reported")
	}
}
