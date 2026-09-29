package cleanup

import (
	"context"
	"path/filepath"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

// Rule discovers removable items of one kind, such as Xcode DerivedData or stale node_modules.
// Rules only locate items; the Scanner measures them.
type Rule interface {
	Name() string
	Category() Category
	Scan(ctx context.Context, env Env) ([]Item, error)
}

// Env is everything a rule may depend on, so rules can run against a fake home in tests.
type Env struct {
	Home             string
	Now              time.Time
	StaleAfter       time.Duration
	ProjectDirs      []string
	LargeFileMinSize int64
	Runner           macos.Runner
}

// InHome joins elem onto the home directory.
func (e Env) InHome(elem ...string) string {
	return filepath.Join(append([]string{e.Home}, elem...)...)
}

// IsStale reports whether t is older than the staleness threshold. A zero time is not stale.
func (e Env) IsStale(t time.Time) bool {
	return !t.IsZero() && e.Now.Sub(t) > e.StaleAfter
}
