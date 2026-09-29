package todos

import (
	"context"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
)

// repo is what items need from their repository: whose lines are yours, and where commits are on GitHub.
type repo struct {
	email string
	// github is owner/name, empty when the repository has no GitHub remote.
	github   string
	unpushed map[string]bool
}

func readRepo(ctx context.Context, runner macos.Runner, dir string) repo {
	r := repo{unpushed: map[string]bool{}}
	if out, err := git(ctx, runner, dir, "config", "user.email"); err == nil {
		r.email = strings.ToLower(strings.TrimSpace(string(out)))
	}
	if out, err := git(ctx, runner, dir, "remote", "get-url", "origin"); err == nil {
		r.github, _ = pulls.RepoFromURL(strings.TrimSpace(string(out)))
	}
	if out, err := git(ctx, runner, dir, "rev-list", "HEAD", "--not", "--remotes"); err == nil {
		for _, hash := range strings.Fields(string(out)) {
			r.unpushed[hash] = true
		}
	}
	return r
}

// finish marks the item as yours or not and links its commit when GitHub has it.
func (r repo) finish(item *Item) {
	item.Mine = item.Uncommitted || (r.email != "" && strings.EqualFold(item.Email, r.email))
	if item.Commit != "" && r.github != "" && !r.unpushed[item.Commit] {
		item.CommitURL = "https://github.com/" + r.github + "/commit/" + item.Commit
	}
}
