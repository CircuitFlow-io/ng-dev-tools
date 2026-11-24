package rules

import (
	"cmp"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
)

const (
	nvmVersionsDir = ".nvm/versions/node"
	nvmAliasDir    = ".nvm/alias"
	nvmGroup       = "Node versions (nvm)"
	// maxAliasHops stops alias chains such as default -> lts/* -> lts/krypton -> v24.21.0 from looping.
	maxAliasHops = 8
)

// nvmRule offers Node versions installed with nvm other than the newest and the default.
// They are for review: each one carries its own global npm packages.
type nvmRule struct{}

func (nvmRule) Name() string               { return "Old Node versions" }
func (nvmRule) Category() cleanup.Category { return dev }

func (nvmRule) Scan(_ context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	versions := installedNodeVersions(env.InHome(nvmVersionsDir))
	if len(versions) < 2 {
		return nil, nil
	}
	keep := map[string]bool{versions[len(versions)-1]: true}
	if def := defaultNodeVersion(env, versions); def != "" {
		keep[def] = true
	}
	var items []cleanup.Item
	for _, version := range versions {
		if keep[version] {
			continue
		}
		path := filepath.Join(env.InHome(nvmVersionsDir), version)
		item := cleanup.PathItem(dev, cleanup.SafetyReview, "Node "+version+" (nvm)", path)
		item.LastUsed = fsx.ModTime(path)
		item.Group = nvmGroup
		items = append(items, item)
	}
	return items, nil
}

// installedNodeVersions lists folders like "v24.13.0", oldest first.
func installedNodeVersions(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var versions []string
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), "v") {
			versions = append(versions, entry.Name())
		}
	}
	slices.SortFunc(versions, compareNodeVersions)
	return versions
}

// defaultNodeVersion follows nvm's default alias to an installed version.
func defaultNodeVersion(env cleanup.Env, installed []string) string {
	name := readAlias(env, "default")
	for range maxAliasHops {
		if name == "" || slices.Contains(installed, name) {
			return name
		}
		next := readAlias(env, name)
		if next == "" {
			break
		}
		name = next
	}
	return newestMatching(installed, name)
}

func readAlias(env cleanup.Env, name string) string {
	data, err := os.ReadFile(filepath.Join(env.InHome(nvmAliasDir), name))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// newestMatching resolves a partial version such as "24" or "v24.1" the way nvm does.
func newestMatching(installed []string, partial string) string {
	prefix := "v" + strings.TrimPrefix(partial, "v")
	for _, version := range slices.Backward(installed) {
		if version == prefix || strings.HasPrefix(version, prefix+".") {
			return version
		}
	}
	return ""
}

func compareNodeVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := range min(len(pa), len(pb)) {
		if c := cmp.Compare(pa[i], pb[i]); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(pa), len(pb))
}

func versionParts(version string) []int {
	fields := strings.Split(strings.TrimPrefix(version, "v"), ".")
	parts := make([]int, 0, len(fields))
	for _, field := range fields {
		n, _ := strconv.Atoi(field)
		parts = append(parts, n)
	}
	return parts
}
