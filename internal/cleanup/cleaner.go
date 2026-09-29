package cleanup

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

// ErrRootNotAuthorized is returned for root-owned items when sudo was not granted.
var ErrRootNotAuthorized = errors.New("administrator access was not granted")

// CleanProgress is reported after each item is processed.
type CleanProgress struct {
	Done       int
	Total      int
	BytesDone  int64
	BytesTotal int64
	Current    string
}

// CleanResult is the outcome for one item.
type CleanResult struct {
	Item Item
	Err  error
}

// Cleaner permanently removes items.
type Cleaner struct {
	Guard  Guard
	Runner macos.Runner
	// DryRun reports what would be removed without touching anything.
	DryRun bool
	// RootAuthorized says sudo credentials are cached, so root-owned items can use `sudo -n`.
	RootAuthorized bool
	// Log receives one line per item; it may be nil.
	Log io.Writer
}

// Clean removes items one at a time, continuing past failures. It stops early if ctx is cancelled.
func (c Cleaner) Clean(ctx context.Context, items []Item, onProgress func(CleanProgress)) []CleanResult {
	progress := CleanProgress{Total: len(items), BytesTotal: TotalSize(items)}
	results := make([]CleanResult, 0, len(items))
	for _, item := range items {
		if ctx.Err() != nil {
			break
		}
		progress.Current = item.Title
		err := c.remove(ctx, item)
		c.log(item, err)
		results = append(results, CleanResult{Item: item, Err: err})

		progress.Done++
		progress.BytesDone += item.Size
		if onProgress != nil {
			onProgress(progress)
		}
	}
	return results
}

func (c Cleaner) remove(ctx context.Context, item Item) error {
	for _, path := range item.Paths {
		if err := c.Guard.Check(path); err != nil {
			return err
		}
	}
	if c.DryRun {
		return nil
	}
	if len(item.RemoveCommand) > 0 {
		_, err := c.Runner.Run(ctx, item.RemoveCommand[0], item.RemoveCommand[1:]...)
		return err
	}
	if item.NeedsRoot {
		return c.removeAsRoot(ctx, item.Paths)
	}
	return removePaths(item.Paths)
}

func (c Cleaner) removeAsRoot(ctx context.Context, paths []string) error {
	if !c.RootAuthorized {
		return ErrRootNotAuthorized
	}
	args := append([]string{"-n", "rm", "-rf", "--"}, paths...)
	_, err := c.Runner.Run(ctx, "sudo", args...)
	return err
}

func removePaths(paths []string) error {
	var errs []error
	for _, path := range paths {
		errs = append(errs, fsx.RemoveAll(path))
	}
	return errors.Join(errs...)
}

func (c Cleaner) log(item Item, err error) {
	if c.Log == nil {
		return
	}
	status := "deleted"
	switch {
	case err != nil:
		status = "FAILED: " + err.Error()
	case c.DryRun:
		status = "dry-run"
	}
	_, _ = fmt.Fprintf(c.Log, "%s\t%d\t%s\t%s\t%v\n", time.Now().Format(time.RFC3339), item.Size, item.Title, status, item.Paths)
}
