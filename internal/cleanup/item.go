// Package cleanup finds reclaimable disk space on macOS and removes it.
package cleanup

import (
	"fmt"
	"strings"
	"time"
)

// Category groups items by where they come from.
type Category int

const (
	CategoryCaches Category = iota
	CategoryDeveloper
	CategoryApps
	CategoryFiles
)

// AllCategories lists every category in display order.
var AllCategories = []Category{CategoryCaches, CategoryDeveloper, CategoryApps, CategoryFiles}

var categoryInfo = map[Category]struct{ key, title string }{
	CategoryCaches:    {"caches", "System & app caches"},
	CategoryDeveloper: {"dev", "Developer tooling"},
	CategoryApps:      {"apps", "Unused applications"},
	CategoryFiles:     {"files", "Large old files"},
}

// String returns the human-readable category title.
func (c Category) String() string { return categoryInfo[c].title }

// Key returns the short name used on the command line.
func (c Category) Key() string { return categoryInfo[c].key }

// ParseCategory resolves a command-line key such as "dev" to a Category.
func ParseCategory(key string) (Category, error) {
	for _, c := range AllCategories {
		if c.Key() == strings.ToLower(strings.TrimSpace(key)) {
			return c, nil
		}
	}
	return 0, fmt.Errorf("unknown category %q (want one of: caches, dev, apps, files)", key)
}

// Safety says whether an item can be removed without a second thought.
type Safety int

const (
	// SafetySafe items are regenerated automatically by the tool that created them.
	SafetySafe Safety = iota
	// SafetyReview items may hold something the user wants to keep.
	SafetyReview
)

// Item is one removable thing found during a scan.
type Item struct {
	Title    string
	Detail   string
	Category Category
	Safety   Safety
	// Paths are the filesystem locations the item occupies. They are measured, de-duplicated,
	// checked by the Guard and, unless RemoveCommand is set, deleted.
	Paths []string
	// RemoveCommand, when set, is run instead of deleting Paths (for example `xcrun simctl delete`).
	RemoveCommand []string
	Size          int64
	LastUsed      time.Time
	NeedsRoot     bool
}

// PathItem builds an item that is removed by deleting its paths.
func PathItem(category Category, safety Safety, title string, paths ...string) Item {
	detail := paths[0]
	if extra := len(paths) - 1; extra > 0 {
		detail = fmt.Sprintf("%s (+%d more)", paths[0], extra)
	}
	return Item{
		Title:    title,
		Detail:   detail,
		Category: category,
		Safety:   safety,
		Paths:    paths,
	}
}

// TotalSize sums the size of items.
func TotalSize(items []Item) int64 {
	var total int64
	for _, item := range items {
		total += item.Size
	}
	return total
}
