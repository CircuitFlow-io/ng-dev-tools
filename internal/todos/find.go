package todos

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const (
	maxParallelRepos = 8
	maxParallelBlame = 4
	// maxLineBytes skips minified code, where a marker is not a note anyone left.
	maxLineBytes = 400
	// grepNoMatch is git grep's exit status when nothing matches.
	grepNoMatch = 1
)

// grepExcludes leave out dependencies, build output and generated files that are committed anyway.
var grepExcludes = []string{
	":(exclude,glob)**/node_modules/**", ":(exclude,glob)**/vendor/**", ":(exclude,glob)**/dist/**",
	":(exclude,glob)**/build/**", ":(exclude,glob)**/Pods/**", ":(exclude,glob)**/*.min.js",
	":(exclude,glob)**/*.map", ":(exclude,glob)**/*lock.json", ":(exclude,glob)**/*lock.yaml", ":(exclude,glob)**/*.lock",
}

// FindAll finds the markers in every git repository among the projects in root, oldest first.
// A repository that cannot be read is reported in errs by project name and left out.
func FindAll(ctx context.Context, runner macos.Runner, root string) (items []Item, errs map[string]error, err error) {
	dirs, err := projects.Dirs(root)
	if err != nil {
		return nil, nil, err
	}
	dirs = slices.DeleteFunc(dirs, func(dir string) bool { return projects.GitDir(dir) == "" })
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
	items, err := grep(ctx, runner, project, dir)
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

// grep lists the marker comments in the tracked files and the untracked ones git does not ignore.
func grep(ctx context.Context, runner macos.Runner, project, dir string) ([]Item, error) {
	args := append([]string{"grep", "-I", "-n", "-z", "--untracked", "-E", `(TODO|FIXME|HACK)`, "--", "."}, grepExcludes...)
	out, err := git(ctx, runner, dir, args...)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == grepNoMatch {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var items []Item
	for line := range bytes.Lines(out) {
		if item, ok := parseGrepLine(string(bytes.TrimSuffix(line, []byte("\n")))); ok {
			item.Project, item.Dir = project, dir
			items = append(items, item)
		}
	}
	return items, nil
}

// parseGrepLine reads "file\x00line\x00text", keeping it when the text is a marker comment.
func parseGrepLine(line string) (Item, bool) {
	parts := strings.SplitN(line, "\x00", 3)
	if len(parts) != 3 || len(parts[2]) > maxLineBytes {
		return Item{}, false
	}
	number, err := strconv.Atoi(parts[1])
	if err != nil {
		return Item{}, false
	}
	marker, note, ok := parseMarker(parts[0], parts[2])
	if !ok {
		return Item{}, false
	}
	return Item{File: parts[0], Line: number, Marker: marker, Note: note}, true
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
