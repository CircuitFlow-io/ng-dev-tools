package cleanup

import (
	"cmp"
	"context"
	"fmt"
	"runtime"
	"slices"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
)

// minReportableSize hides items too small to be worth a line in the list.
const minReportableSize = 100 * 1024

// ScanPhase is the stage a scan is in.
type ScanPhase int

const (
	PhaseDiscovering ScanPhase = iota
	PhaseMeasuring
)

// ScanProgress is reported while a scan runs. Done and Total count rules while discovering
// and items while measuring.
type ScanProgress struct {
	Phase      ScanPhase
	Done       int
	Total      int
	Current    string
	ItemsFound int
	BytesFound int64
}

// ScanResult holds the items found and the rules that could not finish.
type ScanResult struct {
	Items    []Item
	Warnings []error
}

// Scanner runs rules and measures what they find.
type Scanner struct {
	Rules []Rule
	Env   Env
}

// Scan discovers items with every rule, removes overlaps, measures them and returns them
// grouped by category and sorted by size. onProgress may be nil and is never called concurrently.
func (s Scanner) Scan(ctx context.Context, onProgress func(ScanProgress)) ScanResult {
	report := newProgressReporter(onProgress)
	items, warnings := s.discover(ctx, report)
	items = dedupe(items)
	items = measure(ctx, items, report)
	items = slices.DeleteFunc(items, func(item Item) bool { return item.Size < minReportableSize })
	sortItems(items)
	return ScanResult{Items: items, Warnings: warnings}
}

func (s Scanner) discover(ctx context.Context, report *progressReporter) ([]Item, []error) {
	found := make([][]Item, len(s.Rules))
	errs := make([]error, len(s.Rules))
	report.start(PhaseDiscovering, len(s.Rules))

	var g errgroup.Group
	g.SetLimit(runtime.NumCPU())
	for i, rule := range s.Rules {
		g.Go(func() error {
			report.begin(rule.Name())
			items, err := rule.Scan(ctx, s.Env)
			found[i] = items
			if err != nil {
				errs[i] = fmt.Errorf("%s: %w", rule.Name(), err)
			}
			report.finish(rule.Name(), len(items), 0)
			return nil
		})
	}
	_ = g.Wait()

	return slices.Concat(found...), compact(errs)
}

func measure(ctx context.Context, items []Item, report *progressReporter) []Item {
	report.start(PhaseMeasuring, len(items))

	var g errgroup.Group
	g.SetLimit(runtime.NumCPU())
	for i := range items {
		g.Go(func() error {
			report.begin(items[i].Title)
			measureItem(ctx, &items[i], report.addBytes)
			report.finish(items[i].Title, 0, presetSize(items[i]))
			return nil
		})
	}
	_ = g.Wait()
	return items
}

// measureItem sets the item's size from its paths, streaming bytes to onBytes as they are counted.
func measureItem(ctx context.Context, item *Item, onBytes func(int64)) {
	if len(item.Paths) == 0 {
		return
	}
	var size int64
	for _, path := range item.Paths {
		size += fsx.DiskUsageWithProgress(ctx, path, onBytes)
		item.NeedsRoot = item.NeedsRoot || fsx.NeedsRoot(path)
	}
	item.Size = size
}

// presetSize is the size a rule reported itself, for items that have no paths to measure.
func presetSize(item Item) int64 {
	if len(item.Paths) > 0 {
		return 0
	}
	return item.Size
}

func sortItems(items []Item) {
	slices.SortStableFunc(items, func(a, b Item) int {
		return cmp.Or(cmp.Compare(a.Category, b.Category), cmp.Compare(b.Size, a.Size))
	})
}

func compact(errs []error) []error {
	return slices.DeleteFunc(errs, func(err error) bool { return err == nil })
}

// progressReporter serialises progress updates from concurrent workers.
type progressReporter struct {
	mu       sync.Mutex
	onUpdate func(ScanProgress)
	state    ScanProgress
	// inFlight lists work in start order; the oldest unfinished task is shown as current,
	// because it is usually the slow one everyone is waiting for.
	inFlight []string
}

func newProgressReporter(onUpdate func(ScanProgress)) *progressReporter {
	return &progressReporter{onUpdate: onUpdate}
}

func (r *progressReporter) start(phase ScanPhase, total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.Phase, r.state.Done, r.state.Total = phase, 0, total
	r.emit()
}

func (r *progressReporter) begin(task string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inFlight = append(r.inFlight, task)
	r.emit()
}

func (r *progressReporter) finish(task string, items int, bytes int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if i := slices.Index(r.inFlight, task); i >= 0 {
		r.inFlight = slices.Delete(r.inFlight, i, i+1)
	}
	r.state.Done++
	r.state.ItemsFound += items
	r.state.BytesFound += bytes
	r.emit()
}

func (r *progressReporter) addBytes(n int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.state.BytesFound += n
	r.emit()
}

func (r *progressReporter) emit() {
	if r.onUpdate == nil {
		return
	}
	r.state.Current = ""
	if len(r.inFlight) > 0 {
		r.state.Current = r.inFlight[0]
	}
	r.onUpdate(r.state)
}
