package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nasserghiasi/ng-dev-tools/internal/fsx"
)

const (
	minPnpmMajor    = 11
	nvmInstallFix   = "curl -o- https://raw.githubusercontent.com/nvm-sh/nvm/master/install.sh | bash"
	nodeUpgradeFix  = "nvm install --lts && nvm alias default 'lts/*'"
	pnpmInstallFix  = "npm install -g pnpm@latest"
	corepackPnpmFix = "corepack disable pnpm && npm install -g pnpm@latest"
)

func nodeChecks() []Check {
	return []Check{
		{Name: "nvm", Group: GroupNode, Run: checkNvm},
		{Name: "Node.js", Group: GroupNode, Run: checkNode},
		{Name: "pnpm", Group: GroupNode, Run: checkPnpm},
	}
}

func (e Env) nvmDir() string {
	if dir := e.Getenv("NVM_DIR"); dir != "" {
		return dir
	}
	return e.homePath(".nvm")
}

func checkNvm(_ context.Context, env Env) Result {
	dir := env.nvmDir()
	if !fsx.Exists(filepath.Join(dir, "nvm.sh")) {
		return fail("not installed", nvmInstallFix)
	}
	var pkg struct {
		Version string `json:"version"`
	}
	data, err := os.ReadFile(filepath.Join(dir, "package.json"))
	if err != nil || json.Unmarshal(data, &pkg) != nil {
		return pass("installed")
	}
	return pass(pkg.Version)
}

func checkNode(ctx context.Context, env Env) Result {
	current, err := env.version(ctx, "node", "--version")
	if err != nil {
		return missing(err, nodeUpgradeFix)
	}
	lts, err := env.Releases.NodeLTS(ctx)
	return judgeNode(current, lts, err).with(env.nvmDefaultNote()...)
}

func judgeNode(current, lts Version, err error) Result {
	if err != nil {
		return compareWithLatest(current, lts, err, nodeUpgradeFix)
	}
	switch {
	case current.Major < lts.Major:
		return fail(fmt.Sprintf("%s, the latest LTS is %s", current, lts), nodeUpgradeFix)
	case current.Major > lts.Major:
		return warn(fmt.Sprintf("%s is newer than the LTS line (%s)", current, lts), nodeUpgradeFix)
	case current.Less(lts):
		return warn(fmt.Sprintf("%s, %s is available", current, lts), nodeUpgradeFix)
	default:
		return pass(current.String() + ", the latest LTS")
	}
}

// nvmDefaultNote shows which version new shells start with, since it can differ from the active one.
func (e Env) nvmDefaultNote() []string {
	alias, err := os.ReadFile(filepath.Join(e.nvmDir(), "alias", "default"))
	if err != nil {
		return nil
	}
	return []string{"nvm default: " + strings.TrimSpace(string(alias))}
}

func checkPnpm(ctx context.Context, env Env) Result {
	current, err := env.version(ctx, "pnpm", "--version")
	if err != nil {
		return pnpmMissing(err)
	}
	if current.Major < minPnpmMajor {
		return fail(fmt.Sprintf("%s, need %d or newer", current, minPnpmMajor), pnpmInstallFix)
	}
	latest, err := env.Releases.Npm(ctx, "pnpm")
	return compareWithLatest(current, latest, err, pnpmInstallFix)
}

func pnpmMissing(err error) Result {
	if strings.Contains(err.Error(), "corepack") {
		return fail("the corepack shim for pnpm crashes", corepackPnpmFix).with(errorLine(err))
	}
	return missing(err, pnpmInstallFix)
}
