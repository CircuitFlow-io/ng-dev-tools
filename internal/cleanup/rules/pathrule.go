// Package rules holds the catalogue of things ngt knows how to clean.
package rules

import (
	"context"
	"path/filepath"
	"slices"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/fsx"
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
}

func (r PathRule) Name() string               { return r.Title }
func (r PathRule) Category() cleanup.Category { return r.Cat }

func (r PathRule) Scan(_ context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	matches := r.matches(env)
	if len(matches) == 0 {
		return nil, nil
	}
	if !r.Separate {
		return []cleanup.Item{cleanup.PathItem(r.Cat, r.Safety, r.Title, matches...)}, nil
	}
	items := make([]cleanup.Item, 0, len(matches))
	for _, match := range matches {
		items = append(items, cleanup.PathItem(r.Cat, r.Safety, r.Title+": "+filepath.Base(match), match))
	}
	return items, nil
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

func separate(r PathRule) PathRule {
	r.Separate = true
	return r
}

func staleOnly(r PathRule) PathRule {
	r.StaleOnly = true
	return r
}
