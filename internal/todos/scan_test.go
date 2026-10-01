package todos

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

func TestFindSkipsWhatCannotBeANote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet", "api")
	dir := filepath.Join(root, "api")
	write(t, dir, "kept.ts", "// TODO: kept\n")
	write(t, dir, "data.ts", strings.Repeat("export const x = 1\n", maxFileBytes/10)+"// TODO in a huge file\n")
	write(t, dir, "bundle.js", strings.Repeat("a", maxLineBytes)+" // TODO minified\n// TODO: after a long line\n")
	write(t, dir, "image.ts", "\x00// TODO in a binary file\n")
	if err := syscall.Mkfifo(filepath.Join(dir, "pipe.ts"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(dir, "kept.ts"), filepath.Join(dir, "link.ts")); err != nil {
		t.Fatal(err)
	}

	done := make(chan []Item)
	go func() {
		items, err := Find(context.Background(), macos.ExecRunner{}, "api", dir)
		if err != nil {
			t.Error(err)
		}
		done <- items
	}()
	var items []Item
	select {
	case items = <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Find hung, probably opening the named pipe")
	}
	var got []string
	for _, i := range items {
		got = append(got, i.Location())
	}
	slices.Sort(got)
	if want := []string{"bundle.js:2", "kept.ts:1"}; !slices.Equal(got, want) {
		t.Errorf("items = %q, want %q", got, want)
	}
}

// countingRunner records the most commands it ever ran at once.
type countingRunner struct {
	macos.Runner
	running, most atomic.Int32
}

func (r *countingRunner) Run(context.Context, string, ...string) ([]byte, error) {
	now := r.running.Add(1)
	for {
		most := r.most.Load()
		if now <= most || r.most.CompareAndSwap(most, now) {
			break
		}
	}
	time.Sleep(time.Millisecond)
	r.running.Add(-1)
	return nil, nil
}

func TestLimitedRunnerRunsAtMostNAtOnce(t *testing.T) {
	counting := &countingRunner{}
	runner := limitRunner(counting, 2)
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { _, _ = runner.Run(context.Background(), "git") })
	}
	wg.Wait()
	if most := counting.most.Load(); most > 2 {
		t.Errorf("ran %d at once, want at most 2", most)
	}
}
