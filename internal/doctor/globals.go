package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const maxListedPackages = 8

// misplacedGlobals are packages that belong in each project (run through npx) or are deprecated.
var misplacedGlobals = []string{
	"expo", "expo-cli", "react-native", "react-native-cli", "@react-native-community/cli", "create-react-app",
}

func globalsChecks() []Check {
	return []Check{
		{Name: "Misplaced npm globals", Group: GroupGlobals, Run: checkMisplacedGlobals},
		{Name: "Outdated npm globals", Group: GroupGlobals, NeedsNetwork: true, Run: checkOutdatedGlobals},
		{Name: "Homebrew packages", Group: GroupGlobals, Run: checkBrewOutdated},
	}
}

// npmJSON runs an npm command that prints JSON. npm exits non-zero when it has findings to
// report, so the output is used whenever there is some.
func (e Env) npmJSON(ctx context.Context, into any, args ...string) error {
	out, err := e.Runner.Run(ctx, "npm", args...)
	if len(bytes.TrimSpace(out)) > 0 {
		return json.Unmarshal(out, into)
	}
	return err
}

func checkMisplacedGlobals(ctx context.Context, env Env) Result {
	if !env.installed("npm") {
		return skip("npm is not installed")
	}
	var listing struct {
		Dependencies map[string]json.RawMessage `json:"dependencies"`
	}
	if err := env.npmJSON(ctx, &listing, "ls", "-g", "--depth=0", "--json"); err != nil {
		return warn("could not list global packages", "").with(errorLine(err))
	}
	var found []string
	for _, name := range misplacedGlobals {
		if _, ok := listing.Dependencies[name]; ok {
			found = append(found, name)
		}
	}
	if len(found) > 0 {
		return warn(strings.Join(found, ", ")+" installed globally; use them through npx in each project",
			"npm uninstall -g "+strings.Join(found, " "))
	}
	return pass(ui.Count(len(listing.Dependencies), "global package"))
}

type outdatedPackage struct {
	Current string `json:"current"`
	Latest  string `json:"latest"`
}

func checkOutdatedGlobals(ctx context.Context, env Env) Result {
	if !env.installed("npm") {
		return skip("npm is not installed")
	}
	var outdated map[string]outdatedPackage
	if err := env.npmJSON(ctx, &outdated, "outdated", "-g", "--json"); err != nil {
		return warn("could not check for updates", "").with(errorLine(err))
	}
	if len(outdated) == 0 {
		return pass("all up to date")
	}
	names := sortedKeys(outdated)
	details := make([]string, 0, len(names))
	for _, name := range names {
		details = append(details, fmt.Sprintf("%s %s → %s", name, outdated[name].Current, outdated[name].Latest))
	}
	return warn(ui.Count(len(outdated), "outdated package"), "npm update -g").with(limitList(details)...)
}

func checkBrewOutdated(ctx context.Context, env Env) Result {
	if !env.installed("brew") {
		return skip("Homebrew is not installed")
	}
	out, err := env.output(ctx, "env", "HOMEBREW_NO_AUTO_UPDATE=1", "brew", "outdated", "--quiet")
	if err != nil {
		return warn("could not list outdated packages", "").with(errorLine(err))
	}
	if out == "" {
		return pass("all up to date")
	}
	names := strings.Fields(out)
	return warn(ui.Count(len(names), "outdated package"), "brew upgrade").with(limitList(names)...)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// limitList keeps long lists readable by summarising the tail.
func limitList(items []string) []string {
	if len(items) <= maxListedPackages {
		return items
	}
	return append(slices.Clone(items[:maxListedPackages]), fmt.Sprintf("and %d more", len(items)-maxListedPackages))
}
