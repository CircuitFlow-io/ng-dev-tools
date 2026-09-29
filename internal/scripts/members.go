package scripts

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

const (
	pnpmWorkspaceFile = "pnpm-workspace.yaml"
	nodeModules       = "node_modules"
	excludePrefix     = "!"
	anyDepth          = "**"
	// maxMemberDepth bounds the search for members, which a "**" pattern would otherwise take
	// through the whole tree.
	maxMemberDepth = 6
)

type pnpmWorkspace struct {
	Packages             []string `yaml:"packages"`
	EnablePrePostScripts bool     `yaml:"enablePrePostScripts"`
}

func readPNPMWorkspace(root string) (pnpmWorkspace, error) {
	data, err := os.ReadFile(filepath.Join(root, pnpmWorkspaceFile))
	if errors.Is(err, fs.ErrNotExist) {
		return pnpmWorkspace{}, nil
	}
	if err != nil {
		return pnpmWorkspace{}, err
	}
	var settings pnpmWorkspace
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return pnpmWorkspace{}, err
	}
	return settings, nil
}

// memberPatterns are the globs naming a monorepo's packages: pnpm-workspace.yaml's "packages", or
// package.json's "workspaces" as a list or as {"packages": [...]}.
func memberPatterns(settings pnpmWorkspace, m manifest) ([]string, error) {
	if len(settings.Packages) > 0 {
		return settings.Packages, nil
	}
	if len(m.Workspaces) == 0 {
		return nil, nil
	}
	var list []string
	if err := json.Unmarshal(m.Workspaces, &list); err == nil {
		return list, nil
	}
	var object struct {
		Packages []string `json:"packages"`
	}
	if err := json.Unmarshal(m.Workspaces, &object); err != nil {
		return nil, err
	}
	return object.Packages, nil
}

func hasWorkspaces(m manifest) bool {
	patterns, err := memberPatterns(pnpmWorkspace{}, m)
	return err == nil && len(patterns) > 0
}

// findMembers returns the folders, relative to root, that hold a package.json and match patterns.
// A pattern starting with "!" excludes what it matches.
func findMembers(root string, patterns []string) ([]string, error) {
	include, exclude := splitPatterns(patterns)
	if len(include) == 0 {
		return nil, nil
	}
	var members []string
	err := filepath.WalkDir(root, func(dir string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		if dir == root {
			return nil
		}
		rel, _ := filepath.Rel(root, dir)
		rel = filepath.ToSlash(rel)
		if skipsDir(d.Name()) || strings.Count(rel, "/") >= maxMemberDepth {
			return filepath.SkipDir
		}
		segments := strings.Split(rel, "/")
		if matchesAny(include, segments) && !matchesAny(exclude, segments) && exists(filepath.Join(dir, manifestFile)) {
			members = append(members, rel)
		}
		if !descendsAny(include, segments) {
			return filepath.SkipDir
		}
		return nil
	})
	return members, err
}

func skipsDir(name string) bool {
	return name == nodeModules || strings.HasPrefix(name, ".")
}

func splitPatterns(patterns []string) (include, exclude []string) {
	for _, p := range patterns {
		negated := strings.HasPrefix(p, excludePrefix)
		p = normalizePattern(strings.TrimPrefix(p, excludePrefix))
		if negated {
			exclude = append(exclude, p)
		} else {
			include = append(include, p)
		}
	}
	return include, exclude
}

func normalizePattern(p string) string {
	return strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(p), "./"), "/")
}

func matchesAny(patterns []string, segments []string) bool {
	for _, p := range patterns {
		if matchPattern(strings.Split(p, "/"), segments) {
			return true
		}
	}
	return false
}

func descendsAny(patterns []string, segments []string) bool {
	for _, p := range patterns {
		if canDescend(strings.Split(p, "/"), segments) {
			return true
		}
	}
	return false
}

// canDescend reports whether pattern could match a folder below segments, so the walk only enters
// folders that may hold members.
func canDescend(pattern, segments []string) bool {
	switch {
	case len(pattern) == 0:
		return false
	case pattern[0] == anyDepth:
		return true
	case len(segments) == 0:
		return true
	}
	ok, err := path.Match(pattern[0], segments[0])
	return err == nil && ok && canDescend(pattern[1:], segments[1:])
}

// matchPattern matches path segments against glob segments, where "**" stands for any number of
// segments, including none.
func matchPattern(pattern, segments []string) bool {
	if len(pattern) == 0 {
		return len(segments) == 0
	}
	if pattern[0] == anyDepth {
		for skip := 0; skip <= len(segments); skip++ {
			if matchPattern(pattern[1:], segments[skip:]) {
				return true
			}
		}
		return false
	}
	if len(segments) == 0 {
		return false
	}
	ok, err := path.Match(pattern[0], segments[0])
	return err == nil && ok && matchPattern(pattern[1:], segments[1:])
}
