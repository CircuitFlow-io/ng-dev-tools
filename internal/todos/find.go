package todos

import (
	"context"
	"path/filepath"
	"slices"
	"strconv"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const (
	maxParallelRepos = 4
	maxParallelBlame = 4
	// maxGitProcesses bounds the git processes running at once across every repository: one git
	// blame in a large repository can take hundreds of megabytes.
	maxGitProcesses = 6
)

// FindAll finds the markers in every git repository among the projects in root, oldest first.
// A repository that cannot be read is reported in errs by project name and left out.
func FindAll(ctx context.Context, runner macos.Runner, root string) (items []Item, errs map[string]error, err error) {
	dirs, err := projects.Dirs(root)
	if err != nil {
		return nil, nil, err
	}
	dirs = slices.DeleteFunc(dirs, func(dir string) bool { return projects.GitDir(dir) == "" })
	runner = limitRunner(runner, maxGitProcesses)
	found := make([][]Item, len(dirs))
	failed := make([]error, len(dirs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelRepos)
	for i, dir := range dirs {
		g.Go(func() error {
			found[i], failed[i] = Find(gctx, runner, relativeName(root, dir), dir)
			return nil
		})
	}
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	errs = map[string]error{}
	for i, err := range failed {
		if err != nil {
			errs[relativeName(root, dirs[i])] = err
		}
	}
	items = slices.Concat(found...)
	Sort(items)
	return items, errs, nil
}

// Find finds the markers in one repository and dates them with git blame. It only reads.
func Find(ctx context.Context, runner macos.Runner, project, dir string) ([]Item, error) {
	items, err := findMarkers(ctx, runner, project, dir)
	if err != nil || len(items) == 0 {
		return items, err
	}
	r := readRepo(ctx, runner, dir)
	byFile := map[string][]int{}
	for i, item := range items {
		byFile[item.File] = append(byFile[item.File], i)
	}
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelBlame)
	for file, indexes := range byFile {
		g.Go(func() error {
			blameFile(gctx, runner, dir, file, items, indexes)
			return nil
		})
	}
	_ = g.Wait()
	for i := range items {
		r.finish(&items[i])
	}
	return items, ctx.Err()
}

func git(ctx context.Context, runner macos.Runner, dir string, args ...string) ([]byte, error) {
	return runner.Run(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
}

func relativeName(root, dir string) string {
	name, err := filepath.Rel(root, dir)
	if err != nil {
		return filepath.Base(dir)
	}
	return name
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
