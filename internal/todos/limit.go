package todos

import (
	"context"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

// limitedRunner runs at most cap(slots) commands at once, however many goroutines ask.
type limitedRunner struct {
	macos.Runner
	slots chan struct{}
}

func limitRunner(runner macos.Runner, n int) limitedRunner {
	return limitedRunner{Runner: runner, slots: make(chan struct{}, n)}
}

func (r limitedRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	select {
	case r.slots <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-r.slots }()
	return r.Runner.Run(ctx, name, args...)
}
