package claudesessions

import (
	"slices"
	"strings"
	"unicode"
)

const (
	modelPrefix = "claude-"
	// syntheticModel marks messages Claude Code wrote itself, such as API errors.
	syntheticModel   = "<synthetic>"
	snapshotDateSize = len("20251001")
)

var modelFamilies = []string{"opus", "sonnet", "haiku", "fable"}

// ModelName shortens a model id for display: "claude-opus-5-5" is "Opus 5.5" and
// "claude-haiku-4-5-20251001" is "Haiku 4.5". Ids it does not recognise are returned as they are.
func ModelName(id string) string {
	base, variant, _ := strings.Cut(id, "[")
	rest, ok := strings.CutPrefix(base, modelPrefix)
	if !ok {
		return id
	}
	var family string
	var version []string
	for _, part := range strings.Split(rest, "-") {
		switch {
		case slices.Contains(modelFamilies, part):
			family = strings.ToUpper(part[:1]) + part[1:]
		case isVersionNumber(part):
			version = append(version, part)
		}
	}
	if family == "" {
		return id
	}
	name := strings.TrimSpace(family + " " + strings.Join(version, "."))
	if variant != "" {
		name += " [" + variant
	}
	return name
}

// isVersionNumber accepts the short numbers of a version and not a snapshot date.
func isVersionNumber(part string) bool {
	if part == "" || len(part) >= snapshotDateSize {
		return false
	}
	return !strings.ContainsFunc(part, func(r rune) bool { return !unicode.IsDigit(r) })
}
