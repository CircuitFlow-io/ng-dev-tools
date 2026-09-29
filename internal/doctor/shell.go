package doctor

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	minOpenFiles = 10240
	// applePathPrefix marks PATH entries macOS adds for its own tools; they exist only when needed.
	applePathPrefix = "/var/run/com.apple.security.cryptexd/"
	pathMissing     = "missing"
	pathDuplicate   = "duplicate"
)

func shellChecks() []Check {
	return []Check{
		{Name: "UTF-8 locale", Group: GroupShell, Run: checkLocale},
		{Name: "PATH", Group: GroupShell, Run: checkPath},
		{Name: "Node.js installs", Group: GroupShell, Run: checkNodeInstalls},
		{Name: "Open files limit", Group: GroupShell, Run: checkOpenFiles},
	}
}

func checkLocale(_ context.Context, env Env) Result {
	for _, key := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		value := env.Getenv(key)
		if value == "" {
			continue
		}
		if isUTF8(value) {
			return pass(key + "=" + value)
		}
		return warn(key+"="+value+", CocoaPods needs UTF-8", "add to ~/.zshrc: export LANG=en_US.UTF-8")
	}
	return warn("LANG is not set, CocoaPods needs UTF-8", "add to ~/.zshrc: export LANG=en_US.UTF-8")
}

func isUTF8(locale string) bool {
	normalized := strings.ReplaceAll(strings.ToLower(locale), "-", "")
	return strings.Contains(normalized, "utf8")
}

func checkPath(_ context.Context, env Env) Result {
	entries := env.pathEntries()
	var problems []string
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, dir := range entries {
		clean := filepath.Clean(dir)
		if problem := pathProblem(clean, seen); problem != "" {
			problems = append(problems, problem+": "+ui.TildePath(clean, env.Home))
			counts[problem]++
		}
		seen[clean] = true
	}
	if len(problems) > 0 {
		summary := fmt.Sprintf("%d missing, %d duplicated", counts[pathMissing], counts[pathDuplicate])
		return warn(summary, "remove them from ~/.zshrc, ~/.zprofile or /etc/paths.d").with(problems...)
	}
	return pass(ui.Count(len(entries), "entry"))
}

func pathProblem(dir string, seen map[string]bool) string {
	switch {
	case strings.HasPrefix(dir, applePathPrefix):
		return ""
	case seen[dir]:
		return pathDuplicate
	case !fsx.Exists(dir):
		return pathMissing
	default:
		return ""
	}
}

// nodeSource is a way Node.js can be installed.
type nodeSource struct {
	name  string
	paths []string
}

func (e Env) nodeSources() []nodeSource {
	return []nodeSource{
		{"nvm", []string{filepath.Join(e.nvmDir(), "versions", "node")}},
		{"Homebrew", []string{e.systemPath("/opt/homebrew/Cellar/node"), e.systemPath("/usr/local/Cellar/node")}},
		{"Volta", []string{e.homePath(".volta")}},
		{"fnm", []string{e.homePath(".local", "share", "fnm"), e.homePath("Library", "Application Support", "fnm")}},
		{"n", []string{e.systemPath("/usr/local/n")}},
		{"nodejs.org installer", []string{e.systemPath("/usr/local/bin/node")}},
	}
}

func checkNodeInstalls(_ context.Context, env Env) Result {
	var found []string
	for _, source := range env.nodeSources() {
		if slices.ContainsFunc(source.paths, fsx.Exists) {
			found = append(found, source.name)
		}
	}
	switch len(found) {
	case 0:
		return skip("Node.js is not installed")
	case 1:
		return pass("only " + found[0])
	default:
		return warn(fmt.Sprintf("%d installs compete: %s", len(found), strings.Join(found, ", ")),
			"keep nvm and uninstall the others (for Homebrew: brew uninstall node)")
	}
}

func checkOpenFiles(_ context.Context, env Env) Result {
	limit, err := env.Probe.OpenFilesLimit()
	if err != nil {
		return skip("could not read the limit")
	}
	if limit < minOpenFiles {
		return warn(fmt.Sprintf("%d, Metro and watchman need more", limit), fmt.Sprintf("add to ~/.zshrc: ulimit -n %d", minOpenFiles))
	}
	return pass(fmt.Sprint(limit))
}
