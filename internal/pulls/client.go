package pulls

import (
	"context"
	"errors"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const ghLoginHint = "gh auth login"

var (
	ErrNoGH        = errors.New("the GitHub CLI is not installed: brew install gh")
	ErrNotLoggedIn = errors.New("the GitHub CLI is not logged in: run gh auth login")
)

// Load reads the dashboard with one GraphQL request through gh, which holds the GitHub login.
func Load(ctx context.Context, runner macos.Runner) (Dashboard, error) {
	if !runner.Available("gh") {
		return Dashboard{}, ErrNoGH
	}
	out, err := runner.Run(ctx, "gh", "api", "graphql", "-f", "query="+dashboardQuery)
	if err != nil {
		if strings.Contains(err.Error(), ghLoginHint) {
			return Dashboard{}, ErrNotLoggedIn
		}
		return Dashboard{}, err
	}
	return parseDashboard(out)
}
