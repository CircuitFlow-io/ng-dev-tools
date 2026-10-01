package todos

import (
	"context"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	githubLookupTimeout = 5 * time.Second
	noreplyDomain       = "@users.noreply.github.com"
)

// identity is every name and email a line you wrote can carry.
type identity struct {
	emails []string
	names  []string
	// login is your GitHub account, whose noreply addresses are yours.
	login string
}

// gitIdentity is every user.email and user.name set for the repository, global ones included,
// so a repository that overrides your global email still knows the global one.
func gitIdentity(ctx context.Context, runner macos.Runner, dir string) identity {
	return identity{
		emails: configValues(ctx, runner, dir, "user.email"),
		names:  configValues(ctx, runner, dir, "user.name"),
	}
}

func configValues(ctx context.Context, runner macos.Runner, dir, key string) []string {
	out, err := git(ctx, runner, dir, "config", "--get-all", key)
	if err != nil {
		return nil
	}
	return nonEmptyLines(string(out))
}

// githubIdentity is your GitHub login and profile name. Commits GitHub makes for you, such as a
// squash merge, carry your profile name and the commit email set on GitHub, not your git config.
// It is empty when gh is missing, signed out or offline.
func githubIdentity(ctx context.Context, runner macos.Runner) identity {
	ctx, cancel := context.WithTimeout(ctx, githubLookupTimeout)
	defer cancel()
	out, err := runner.Run(ctx, "gh", "api", "user", "--jq", `.login, (.name // "")`)
	if err != nil {
		return identity{}
	}
	lines := nonEmptyLines(string(out))
	if len(lines) == 0 {
		return identity{}
	}
	return identity{login: lines[0], names: lines[1:]}
}

// wrote is whether the line's author is you, by any of your emails or names.
func (id identity) wrote(item Item) bool {
	return containsFold(id.emails, item.Email) || containsFold(id.names, item.Author) || id.ownsNoreply(item.Email)
}

// ownsNoreply matches login@users.noreply.github.com and 123+login@users.noreply.github.com.
func (id identity) ownsNoreply(email string) bool {
	if id.login == "" {
		return false
	}
	local, ok := strings.CutSuffix(strings.ToLower(email), noreplyDomain)
	if !ok {
		return false
	}
	_, user, _ := strings.Cut(local, "+")
	if user == "" {
		user = local
	}
	return strings.EqualFold(user, id.login)
}

func containsFold(values []string, s string) bool {
	if s == "" {
		return false
	}
	for _, v := range values {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}

func nonEmptyLines(s string) []string {
	var lines []string
	for line := range strings.Lines(s) {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}
