package doctor

import (
	"context"
	"path/filepath"

	"github.com/nasserghiasi/ng-dev-tools/internal/fsx"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

const goBinOnPathFix = `add to ~/.zshrc: export PATH="$(go env GOPATH)/bin:$PATH"`

// goTool is a Go development tool and the command that installs it.
type goTool struct {
	name    string
	install string
}

var goTools = []goTool{
	{"gopls", "go install golang.org/x/tools/gopls@latest"},
	{"dlv", "go install github.com/go-delve/delve/cmd/dlv@latest"},
	{"golangci-lint", "brew install golangci-lint"},
	{"gofumpt", "go install mvdan.cc/gofumpt@latest"},
	{"goimports", "go install golang.org/x/tools/cmd/goimports@latest"},
}

func goChecks() []Check {
	checks := []Check{
		{Name: "Go", Group: GroupGo, Run: checkGo},
		{Name: "GOPATH/bin on PATH", Group: GroupGo, Run: checkGoBinOnPath},
	}
	for _, tool := range goTools {
		checks = append(checks, Check{Name: tool.name, Group: GroupGo, Run: tool.check})
	}
	return checks
}

func checkGo(ctx context.Context, env Env) Result {
	current, err := env.version(ctx, "go", "version")
	if err != nil {
		return missing(err, "brew install go")
	}
	latest, err := env.Releases.Go(ctx)
	return compareWithLatest(current, latest, err, "brew upgrade go")
}

// goBin is where `go install` puts binaries.
func (e Env) goBin(ctx context.Context) string {
	if gobin, err := e.output(ctx, "go", "env", "GOBIN"); err == nil && gobin != "" {
		return gobin
	}
	if gopath, err := e.output(ctx, "go", "env", "GOPATH"); err == nil && gopath != "" {
		return filepath.Join(filepath.SplitList(gopath)[0], "bin")
	}
	return e.homePath("go", "bin")
}

func checkGoBinOnPath(ctx context.Context, env Env) Result {
	if !env.installed("go") {
		return skip("Go is not installed")
	}
	bin := env.goBin(ctx)
	if !env.onPath(bin) {
		return warn(ui.TildePath(bin, env.Home)+" is not on PATH", goBinOnPathFix)
	}
	return pass(ui.TildePath(bin, env.Home))
}

func (t goTool) check(ctx context.Context, env Env) Result {
	if path, err := env.LookPath(t.name); err == nil {
		return pass(ui.TildePath(path, env.Home))
	}
	if env.installed("go") && fsx.Exists(filepath.Join(env.goBin(ctx), t.name)) {
		return warn("installed in GOPATH/bin, which is not on PATH", goBinOnPathFix)
	}
	return fail("not installed", t.install)
}
