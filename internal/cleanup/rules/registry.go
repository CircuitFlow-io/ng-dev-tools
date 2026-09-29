package rules

import (
	"slices"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
)

// Default returns every rule. Order matters: when two rules claim the same path the earlier one
// keeps it, so specific rules come before the catch-all cache sweeps.
func Default() []cleanup.Rule {
	return slices.Concat(
		developerRules(),
		[]cleanup.Rule{projectsRule{}},
		cacheRules(),
		catchAllCacheRules(),
		[]cleanup.Rule{unusedAppsRule{}, leftoversRule{}, largeFilesRule{}},
	)
}

// ForCategories keeps only rules in the given categories.
func ForCategories(rules []cleanup.Rule, categories []cleanup.Category) []cleanup.Rule {
	return slices.DeleteFunc(slices.Clone(rules), func(r cleanup.Rule) bool {
		return !slices.Contains(categories, r.Category())
	})
}
