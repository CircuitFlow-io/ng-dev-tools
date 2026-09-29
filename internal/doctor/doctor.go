package doctor

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

const (
	// maxParallelChecks is generous because most checks wait on a subprocess or the network.
	maxParallelChecks = 8
	// DefaultTimeout bounds each check; walking a large cache is the slowest thing doctor does.
	DefaultTimeout = 30 * time.Second
)

// Outcome is a check together with what it found.
type Outcome struct {
	Check    Check
	Result   Result
	Duration time.Duration
}

// Progress is reported as checks finish.
type Progress struct {
	Done    int
	Total   int
	Current string
}

// Doctor runs a set of checks.
type Doctor struct {
	Checks []Check
	Env    Env
	// Offline skips checks that need the network and release lookups.
	Offline bool
	// Timeout bounds each check; zero means DefaultTimeout.
	Timeout time.Duration
}

// Run executes every check concurrently and returns their outcomes in catalog order.
// onProgress may be nil and is never called concurrently.
func (d Doctor) Run(ctx context.Context, onProgress func(Progress)) []Outcome {
	env := d.Env
	if d.Offline {
		env.Releases = OfflineReleases{}
	}
	tracker := newTracker(len(d.Checks), onProgress)
	outcomes := make([]Outcome, len(d.Checks))

	var g errgroup.Group
	g.SetLimit(maxParallelChecks)
	for i, check := range d.Checks {
		g.Go(func() error {
			tracker.begin(check.Name)
			outcomes[i] = d.runOne(ctx, env, check)
			tracker.finish(check.Name)
			return nil
		})
	}
	_ = g.Wait()
	return outcomes
}

func (d Doctor) runOne(ctx context.Context, env Env, check Check) Outcome {
	start := time.Now()
	result := d.result(ctx, env, check)
	return Outcome{Check: check, Result: result, Duration: time.Since(start)}
}

func (d Doctor) result(ctx context.Context, env Env, check Check) Result {
	if ctx.Err() != nil {
		return skip("cancelled")
	}
	if d.Offline && check.NeedsNetwork {
		return skip("needs the network (offline)")
	}
	checkCtx, cancel := context.WithTimeout(ctx, d.timeout())
	defer cancel()

	done := make(chan Result, 1)
	go func() { done <- safeRun(checkCtx, env, check) }()
	select {
	case result := <-done:
		return result
	case <-checkCtx.Done():
		if ctx.Err() != nil {
			return skip("cancelled")
		}
		return warn(fmt.Sprintf("did not finish within %s", d.timeout()), "")
	}
}

func (d Doctor) timeout() time.Duration {
	if d.Timeout > 0 {
		return d.Timeout
	}
	return DefaultTimeout
}

// safeRun turns a panicking check into a failure instead of crashing the whole report.
func safeRun(ctx context.Context, env Env, check Check) (result Result) {
	defer func() {
		if r := recover(); r != nil {
			result = fail("the check crashed", "").with(fmt.Sprint(r))
		}
	}()
	return check.Run(ctx, env)
}

// Select keeps the checks in the given groups, or all of them when groups is empty.
func Select(checks []Check, groups []Group) []Check {
	if len(groups) == 0 {
		return checks
	}
	return slices.DeleteFunc(slices.Clone(checks), func(c Check) bool { return !slices.Contains(groups, c.Group) })
}

// Count returns how many outcomes have the given status.
func Count(outcomes []Outcome, status Status) int {
	var n int
	for _, o := range outcomes {
		if o.Result.Status == status {
			n++
		}
	}
	return n
}

// tracker serialises progress updates from concurrent checks.
type tracker struct {
	mu       sync.Mutex
	onUpdate func(Progress)
	state    Progress
	// inFlight lists running checks in start order; the oldest one is shown as current,
	// because it is the one everyone is waiting for.
	inFlight []string
}

func newTracker(total int, onUpdate func(Progress)) *tracker {
	return &tracker{onUpdate: onUpdate, state: Progress{Total: total}}
}

func (t *tracker) begin(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inFlight = append(t.inFlight, name)
	t.emit()
}

func (t *tracker) finish(name string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if i := slices.Index(t.inFlight, name); i >= 0 {
		t.inFlight = slices.Delete(t.inFlight, i, i+1)
	}
	t.state.Done++
	t.emit()
}

func (t *tracker) emit() {
	if t.onUpdate == nil {
		return
	}
	t.state.Current = ""
	if len(t.inFlight) > 0 {
		t.state.Current = t.inFlight[0]
	}
	t.onUpdate(t.state)
}
