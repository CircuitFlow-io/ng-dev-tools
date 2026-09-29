package ports

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"golang.org/x/sys/unix"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

var (
	ErrProtectedProcess = errors.New("refusing to stop init or ngt itself")
	ErrProcessReplaced  = errors.New("the process exited and its pid now belongs to another process")
	ErrStillRunning     = errors.New("still running after SIGKILL")
	ErrNotPermitted     = errors.New("owned by another user; run with sudo to stop it")
)

const (
	// DefaultGrace is how long a process gets to shut down cleanly before it is killed.
	DefaultGrace = 3 * time.Second
	killWait     = time.Second
	pollInterval = 50 * time.Millisecond
	// startTimeTolerance absorbs the one-second resolution of ps's elapsed time.
	startTimeTolerance = 2 * time.Second
)

// Outcome is how a process ended.
type Outcome int

const (
	// Terminated means the process exited after SIGTERM.
	Terminated Outcome = iota
	// Killed means the process ignored SIGTERM, or Force was set, and got SIGKILL.
	Killed
	// AlreadyExited means the process was gone before it was signalled.
	AlreadyExited
)

// StopResult is the outcome for one process.
type StopResult struct {
	Process Process
	Outcome Outcome
	Err     error
}

// StopProgress is reported after each process is handled.
type StopProgress struct {
	Done  int
	Total int
	Last  string
}

// Stopper ends processes: SIGTERM first, SIGKILL if they are still running after Grace.
type Stopper struct {
	Runner macos.Runner
	Grace  time.Duration
	// Force skips SIGTERM and sends SIGKILL straight away.
	Force bool
}

// StopAll stops processes concurrently, so slow ones share a single grace period.
// onProgress may be nil and is never called concurrently.
func (s Stopper) StopAll(ctx context.Context, processes []Process, onProgress func(StopProgress)) []StopResult {
	results := make([]StopResult, len(processes))
	var mu sync.Mutex
	var wg sync.WaitGroup
	progress := StopProgress{Total: len(processes)}
	for i, p := range processes {
		wg.Go(func() {
			results[i] = s.Stop(ctx, p)
			mu.Lock()
			defer mu.Unlock()
			progress.Done++
			progress.Last = p.Label()
			if onProgress != nil {
				onProgress(progress)
			}
		})
	}
	wg.Wait()
	return results
}

// Stop ends one process after checking that its pid still belongs to it.
func (s Stopper) Stop(ctx context.Context, p Process) StopResult {
	outcome, err := s.stop(ctx, p)
	return StopResult{Process: p, Outcome: outcome, Err: err}
}

func (s Stopper) stop(ctx context.Context, p Process) (Outcome, error) {
	if p.PID <= 1 || p.PID == os.Getpid() {
		return 0, ErrProtectedProcess
	}
	running, err := s.stillRunning(ctx, p)
	if err != nil || !running {
		return AlreadyExited, err
	}
	if !s.Force {
		exited, err := s.signalAndWait(ctx, p.PID, unix.SIGTERM, s.grace())
		if err != nil || exited {
			return Terminated, err
		}
	}
	exited, err := s.signalAndWait(ctx, p.PID, unix.SIGKILL, killWait)
	if err != nil {
		return Killed, err
	}
	if !exited {
		return Killed, ErrStillRunning
	}
	return Killed, nil
}

// stillRunning reports whether p is still alive, and fails if its pid was reused since listing.
func (s Stopper) stillRunning(ctx context.Context, p Process) (bool, error) {
	out, _ := s.Runner.Run(ctx, "ps", "-ww", "-o", "pid=,etime=,comm=", "-p", strconv.Itoa(p.PID))
	info, ok := parseProcessInfo(string(out))[p.PID]
	if !ok {
		return false, nil
	}
	if info.executable != p.Executable || !startedAround(time.Now().Add(-info.elapsed), p.StartedAt) {
		return false, ErrProcessReplaced
	}
	return true, nil
}

func startedAround(actual, listed time.Time) bool {
	if listed.IsZero() {
		return true
	}
	return actual.Sub(listed).Abs() <= startTimeTolerance
}

func (s Stopper) signalAndWait(ctx context.Context, pid int, sig unix.Signal, wait time.Duration) (bool, error) {
	if err := unix.Kill(pid, sig); err != nil {
		return errors.Is(err, unix.ESRCH), signalError(sig, err)
	}
	return waitForExit(ctx, pid, wait)
}

func signalError(sig unix.Signal, err error) error {
	switch {
	case errors.Is(err, unix.ESRCH):
		return nil
	case errors.Is(err, unix.EPERM):
		return ErrNotPermitted
	default:
		return fmt.Errorf("sending %s: %w", unix.SignalName(sig), err)
	}
}

func waitForExit(ctx context.Context, pid int, wait time.Duration) (bool, error) {
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		if !isAlive(pid) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, nil
		case <-ticker.C:
		}
	}
}

func isAlive(pid int) bool {
	return !errors.Is(unix.Kill(pid, 0), unix.ESRCH)
}

func (s Stopper) grace() time.Duration {
	if s.Grace > 0 {
		return s.Grace
	}
	return DefaultGrace
}
