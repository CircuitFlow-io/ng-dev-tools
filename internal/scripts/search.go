package scripts

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

// Scores for how a search word matches a script or package. A script name counts more than a
// package, so "web dev" puts apps/web's dev above a root script called dev:web.
const (
	scriptExact       = 100
	packageExact      = 60
	scriptPrefix      = 50
	packagePrefix     = 40
	scriptSubstring   = 30
	packageSubstring  = 25
	scriptSubsequence = 10
	packageSubseq     = 5
	recentBonus       = 20
)

// Search keeps the targets matching every word of query, best match first. Targets in recent
// rank higher, and ties keep their order.
func Search(targets []Target, recent []Run, query string) []Target {
	words := strings.Fields(strings.ToLower(query))
	type scored struct {
		target Target
		score  int
	}
	var matches []scored
	for _, t := range targets {
		score, ok := scoreTarget(t, words)
		if !ok {
			continue
		}
		if isRecent(t, recent) {
			score += recentBonus
		}
		matches = append(matches, scored{t, score})
	}
	slices.SortStableFunc(matches, func(a, b scored) int { return cmp.Compare(b.score, a.score) })

	found := make([]Target, len(matches))
	for i, m := range matches {
		found[i] = m.target
	}
	return found
}

func scoreTarget(t Target, words []string) (int, bool) {
	total := 0
	for _, word := range words {
		score := max(scoreWord(word, strings.ToLower(t.Script.Name), scriptExact, scriptPrefix, scriptSubstring, scriptSubsequence),
			packageScore(word, t.Package))
		if score == 0 {
			return 0, false
		}
		total += score
	}
	return total, true
}

func packageScore(word string, p Package) int {
	best := 0
	for _, name := range packageNames(p) {
		best = max(best, scoreWord(word, name, packageExact, packagePrefix, packageSubstring, packageSubseq))
	}
	return best
}

// packageNames are the names a package is searched by: its folder, the folder's last part and
// the package name without its scope.
func packageNames(p Package) []string {
	names := []string{strings.ToLower(p.RelDir), strings.ToLower(path.Base(p.RelDir))}
	if p.IsRoot() {
		names = append(names, strings.ToLower(path.Base(p.Dir)))
	}
	if p.Name != "" {
		_, unscoped, _ := strings.Cut(strings.ToLower(p.Name), "/")
		names = append(names, cmp.Or(unscoped, strings.ToLower(p.Name)))
	}
	return names
}

func scoreWord(word, name string, exact, prefix, substring, subsequence int) int {
	switch {
	case name == "":
		return 0
	case name == word:
		return exact
	case strings.HasPrefix(name, word):
		return prefix
	case strings.Contains(name, word):
		return substring
	case projects.Matches(name, word):
		return subsequence
	default:
		return 0
	}
}

func isRecent(t Target, recent []Run) bool {
	return slices.ContainsFunc(recent, func(r Run) bool { return r.Package == t.Package.RelDir && r.Script == t.Script.Name })
}

// Exact finds the single target words name without doubt: the last word is the exact script name
// and the words before it narrow the package. With no package words, the package in current wins
// when it has the script.
func Exact(targets []Target, current string, words []string) (Target, bool) {
	if len(words) == 0 {
		return Target{}, false
	}
	script, packageWords := words[len(words)-1], words[:len(words)-1]
	var candidates []Target
	for _, t := range targets {
		if t.Script.Name == script && matchesPackage(t.Package, packageWords) {
			candidates = append(candidates, t)
		}
	}
	if len(packageWords) == 0 {
		if i := slices.IndexFunc(candidates, func(t Target) bool { return t.Package.RelDir == current }); i >= 0 {
			return candidates[i], true
		}
	}
	if len(candidates) != 1 {
		return Target{}, false
	}
	return candidates[0], true
}

func matchesPackage(p Package, words []string) bool {
	for _, word := range words {
		if packageScore(strings.ToLower(word), p) == 0 {
			return false
		}
	}
	return true
}
