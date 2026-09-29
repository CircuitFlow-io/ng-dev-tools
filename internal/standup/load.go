package standup

import (
	"context"
	"path/filepath"
	"slices"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const maxParallelRepos = 8

// Load gathers what you did in the git repositories among the projects in root and on GitHub
// since since. A zero since means the start of the last day before today you committed on.
func Load(ctx context.Context, runner macos.Runner, root string, since, now time.Time) (Report, error) {
	repos, err := readRepos(ctx, runner, root)
	if err != nil {
		return Report{}, err
	}
	report := Report{Since: since}
	if since.IsZero() {
		report.Since, report.LastWorkedDay = findLastWorkedDay(ctx, runner, repos, now), true
	}

	var (
		work []localWork
		gh   github
	)
	g, gctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		work = loadWork(gctx, runner, repos, report.Since)
		return nil
	})
	g.Go(func() error {
		gh, report.GitHubErr = loadGitHub(gctx, runner, report.Since)
		return nil
	})
	g.Go(func() error {
		statuses, err := gitstatus.LoadAll(gctx, runner, root)
		report.InProgress = inProgress(statuses)
		return err
	})
	if err := g.Wait(); err != nil {
		return Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	report.Projects = build(report.Since, work, gh.authored)
	report.Reviewed = gh.reviewed
	return report, nil
}

func readRepos(ctx context.Context, runner macos.Runner, root string) ([]repo, error) {
	dirs, err := projects.Dirs(root)
	if err != nil {
		return nil, err
	}
	dirs = oneCheckoutPerRepository(dirs)
	repos := make([]repo, len(dirs))
	forEach(ctx, len(dirs), func(ctx context.Context, i int) {
		repos[i] = readRepo(ctx, runner, relativeName(root, dirs[i]), dirs[i])
	})
	return repos, ctx.Err()
}

// oneCheckoutPerRepository keeps the git checkouts among dirs, one per repository: worktrees
// share their repository's branches and commits, so only the main checkout is kept, or the first
// worktree when the main one is elsewhere.
func oneCheckoutPerRepository(dirs []string) []string {
	var main, linked []string
	for _, dir := range dirs {
		gitDir := projects.GitDir(dir)
		switch {
		case gitDir == "":
		case realPath(projects.CommonDir(gitDir)) == realPath(gitDir):
			main = append(main, dir)
		default:
			linked = append(linked, dir)
		}
	}
	seen := map[string]bool{}
	var kept []string
	for _, dir := range slices.Concat(main, linked) {
		repository := realPath(projects.CommonDir(projects.GitDir(dir)))
		if !seen[repository] {
			seen[repository] = true
			kept = append(kept, dir)
		}
	}
	slices.Sort(kept)
	return kept
}

// realPath resolves symlinks, since git writes a worktree's paths resolved (/private/var for
// /var on macOS).
func realPath(path string) string {
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		return resolved
	}
	return filepath.Clean(path)
}

func findLastWorkedDay(ctx context.Context, runner macos.Runner, repos []repo, now time.Time) time.Time {
	times := make([][]time.Time, len(repos))
	forEach(ctx, len(repos), func(ctx context.Context, i int) {
		times[i] = repos[i].commitTimes(ctx, runner, now)
	})
	return lastWorkedDay(slices.Concat(times...), now)
}

func loadWork(ctx context.Context, runner macos.Runner, repos []repo, since time.Time) []localWork {
	work := make([]localWork, len(repos))
	forEach(ctx, len(repos), func(ctx context.Context, i int) {
		work[i] = repos[i].load(ctx, runner, since)
	})
	return work
}

// forEach runs do for 0 to n-1, a few at a time.
func forEach(ctx context.Context, n int, do func(ctx context.Context, i int)) {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelRepos)
	for i := range n {
		g.Go(func() error {
			do(gctx, i)
			return nil
		})
	}
	_ = g.Wait()
}

func relativeName(root, dir string) string {
	name, err := filepath.Rel(root, dir)
	if err != nil {
		return filepath.Base(dir)
	}
	return name
}
