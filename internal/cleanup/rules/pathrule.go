// Package rules holds the catalogue of things ngt knows how to clean.
package rules

import (
	"context"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
)

// PathRule turns well-known locations into items. Patterns are relative to the home
// directory unless absolute and may contain glob wildcards.
type PathRule struct {
	Title    string
	Cat      cleanup.Category
	Safety   cleanup.Safety
	Patterns []string
	// Separate yields one item per match instead of a single item covering every match.
	Separate bool
	// Exclude lists glob patterns for base names that are never matched.
	Exclude []string
	// StaleOnly keeps only matches not modified within the staleness window.
	StaleOnly bool
	// Group lists every item this rule yields under one heading.
	Group string
}

func (r PathRule) Name() string               { return r.Title }
func (r PathRule) Category() cleanup.Category { return r.Cat }

func (r PathRule) Scan(_ context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	matches := r.matches(env)
	if len(matches) == 0 {
		return nil, nil
	}
	if !r.Separate {
		return []cleanup.Item{r.item(r.Title, matches...)}, nil
	}
	items := make([]cleanup.Item, 0, len(matches))
	for _, match := range matches {
		items = append(items, r.item(r.Title+": "+matchLabel(env, match), match))
	}
	return items, nil
}

func (r PathRule) item(title string, paths ...string) cleanup.Item {
	item := cleanup.PathItem(r.Cat, r.Safety, title, paths...)
	item.Group = r.Group
	return item
}

// genericNames are folder names that say nothing about who owns them.
var genericNames = map[string]bool{"logs": true, "log": true, "data": true}

// matchLabel names a match by its folder, prefixed with parent folders while the name is still
// generic, so ".../Cursor/logs" reads as "Cursor/logs" and ".../com.docker.docker/Data/log" as
// "com.docker.docker/Data/log".
func matchLabel(env cleanup.Env, match string) string {
	label, dir := filepath.Base(match), filepath.Dir(match)
	for genericNames[strings.ToLower(firstSegment(label))] && dir != env.Home && dir != filepath.Dir(dir) {
		label = filepath.Base(dir) + "/" + label
		dir = filepath.Dir(dir)
	}
	return label
}

func firstSegment(label string) string {
	segment, _, _ := strings.Cut(label, "/")
	return segment
}

func (r PathRule) matches(env cleanup.Env) []string {
	var matches []string
	for _, pattern := range r.Patterns {
		if !filepath.IsAbs(pattern) {
			pattern = env.InHome(pattern)
		}
		found, _ := filepath.Glob(pattern)
		for _, path := range found {
			if r.accepts(env, path) {
				matches = append(matches, path)
			}
		}
	}
	return matches
}

func (r PathRule) accepts(env cleanup.Env, path string) bool {
	if r.isExcluded(filepath.Base(path)) {
		return false
	}
	if !r.StaleOnly {
		return true
	}
	return env.IsStale(fsx.ModTime(path))
}

func (r PathRule) isExcluded(name string) bool {
	return slices.ContainsFunc(r.Exclude, func(pattern string) bool {
		matched, _ := filepath.Match(pattern, name)
		return matched
	})
}

func excluding(r PathRule, patterns ...string) PathRule {
	r.Exclude = append(r.Exclude, patterns...)
	return r
}

func safe(cat cleanup.Category, title string, patterns ...string) PathRule {
	return PathRule{Title: title, Cat: cat, Safety: cleanup.SafetySafe, Patterns: patterns}
}

func review(cat cleanup.Category, title string, patterns ...string) PathRule {
	return PathRule{Title: title, Cat: cat, Safety: cleanup.SafetyReview, Patterns: patterns}
}

func grouped(group string, r PathRule) PathRule {
	r.Group = group
	return r
}

func separate(r PathRule) PathRule {
	r.Separate = true
	return r
}

func staleOnly(r PathRule) PathRule {
	r.StaleOnly = true
	return r
}
