package gitstatus

import (
	"context"
	"path/filepath"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const maxParallelLoads = 8

// LoadAll reads every git repository among the projects in root, most urgent first. Projects
// without git are left out.
func LoadAll(ctx context.Context, runner macos.Runner, root string) ([]Repo, error) {
	dirs, err := projects.Dirs(root)
	if err != nil {
		return nil, err
	}
	var repoDirs []string
	for _, dir := range dirs {
		if projects.GitDir(dir) != "" {
			repoDirs = append(repoDirs, dir)
		}
	}

	repos := make([]Repo, len(repoDirs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelLoads)
	for i, dir := range repoDirs {
		g.Go(func() error {
			repos[i] = Load(gctx, runner, relativeName(root, dir), dir)
			return nil
		})
	}
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	Sort(repos)
	return repos, nil
}

func relativeName(root, dir string) string {
	name, err := filepath.Rel(root, dir)
	if err != nil {
		return filepath.Base(dir)
	}
	return name
}
