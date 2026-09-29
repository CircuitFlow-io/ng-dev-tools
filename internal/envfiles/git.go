package envfiles

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const commitPrefix = "commit "

// envPathspecs match every env file name in any folder; names are then told apart by classify.
var envPathspecs = []string{":(glob)**/.env", ":(glob)**/.env.*", ":(glob)**/*.env", ":(glob)**/*.env.*"}

// exposure is what git has of a project's local env files, by absolute path.
type exposure struct {
	tracked   []string
	committed []Commit
}

// readGit finds the local env files in dir that git tracks, and the ones it no longer tracks that
// are still in its history. It only reads.
func readGit(ctx context.Context, runner macos.Runner, dir string) (exposure, error) {
	if projects.GitDir(dir) == "" {
		return exposure{}, nil
	}
	tracked, err := git(ctx, runner, dir, append([]string{"ls-files", "-z", "--"}, envPathspecs...)...)
	if err != nil {
		return exposure{}, fmt.Errorf("could not list tracked files: %w", err)
	}
	history, err := git(ctx, runner, dir, append([]string{"log", "--all", "--diff-filter=A", "--relative", "--format=" + commitPrefix + "%h %ct", "--name-only", "--"}, envPathspecs...)...)
	if err != nil {
		return exposure{}, fmt.Errorf("could not read git history: %w", err)
	}
	e := exposure{tracked: secretPaths(dir, strings.Split(string(tracked), "\x00"))}
	e.committed = committedSecrets(dir, string(history), e.tracked)
	return e, nil
}

func git(ctx context.Context, runner macos.Runner, dir string, args ...string) ([]byte, error) {
	return runner.Run(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
}

func secretPaths(root string, paths []string) []string {
	var secret []string
	for _, p := range paths {
		if p != "" && isLocal(filepath.Base(p)) {
			secret = append(secret, filepath.Join(root, p))
		}
	}
	return secret
}

// committedSecrets reads git log output, newest first, into the latest commit that added each
// local env file that is not tracked any more.
func committedSecrets(root, log string, tracked []string) []Commit {
	skip := map[string]bool{}
	for _, p := range tracked {
		skip[p] = true
	}
	var commits []Commit
	var hash string
	var at time.Time
	for line := range strings.Lines(log) {
		line = strings.TrimSpace(line)
		if header, ok := strings.CutPrefix(line, commitPrefix); ok {
			hash, at = parseCommitHeader(header)
			continue
		}
		path := filepath.Join(root, line)
		if line == "" || skip[path] || !isLocal(filepath.Base(line)) {
			continue
		}
		skip[path] = true
		commits = append(commits, Commit{File: path, Hash: hash, At: at})
	}
	return commits
}

func parseCommitHeader(header string) (string, time.Time) {
	hash, seconds, _ := strings.Cut(header, " ")
	unix, err := strconv.ParseInt(seconds, 10, 64)
	if err != nil {
		return hash, time.Time{}
	}
	return hash, time.Unix(unix, 0)
}

// attachExposure puts each tracked or committed file on the set of its folder.
func (p *project) attachExposure(e exposure) {
	for _, path := range e.tracked {
		s := p.setFor(filepath.Dir(path), filepath.Base(path))
		s.Tracked = append(s.Tracked, filepath.Base(path))
	}
	for _, c := range e.committed {
		s := p.setFor(filepath.Dir(c.File), filepath.Base(c.File))
		c.File = filepath.Base(c.File)
		s.Committed = append(s.Committed, c)
	}
}
