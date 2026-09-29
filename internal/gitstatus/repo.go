// Package gitstatus reads the git state of projects: uncommitted changes, sync with the remote,
// stashes, other branches and operations left in progress. It only reads, apart from Fetch.
package gitstatus

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
	"github.com/nasserghiasi/ng-dev-tools/internal/projects"
)

const (
	maxUnpushedListed = 3
	fieldSeparator    = "\x00"
	commitFormat      = "--format=%h%x00%ct%x00%s"
)

// Repo is the state of one repository.
type Repo struct {
	Name     string
	Path     string
	Branch   string
	Detached bool
	// Unborn is a repository without commits yet.
	Unborn   bool
	Upstream string
	// UpstreamGone is an upstream branch that was deleted on the remote.
	UpstreamGone bool
	Ahead        int
	Behind       int
	HasRemote    bool
	Files        []File
	LastCommit   Commit
	// Unpushed counts the commits of HEAD that are on no remote branch; UnpushedCommits lists the newest.
	Unpushed        int
	UnpushedCommits []Commit
	Stashes         []Stash
	// Branches are the other local branches worth a look: behind, ahead, not pushed or without their remote.
	Branches  []Branch
	Operation Operation
	FetchedAt time.Time
	Err       error
}

// Commit is one commit, newest first in lists.
type Commit struct {
	Hash    string
	At      time.Time
	Subject string
}

// Stash is one stash entry.
type Stash struct {
	Ref     string
	At      time.Time
	Message string
}

// HasUpstream reports whether the branch tracks a remote branch that still exists.
func (r Repo) HasUpstream() bool {
	return r.Upstream != "" && !r.UpstreamGone
}

// Load reads the state of the repository in dir. A failure to read it is kept in Repo.Err.
func Load(ctx context.Context, runner macos.Runner, name, dir string) Repo {
	r := Repo{Name: name, Path: dir}
	out, err := git(ctx, runner, dir, "status", "--porcelain=v2", "--branch", "-z")
	if err != nil {
		r.Err = err
		return r
	}
	r.applyStatus(parseStatus(out))

	gitDir := projects.GitDir(dir)
	r.Operation = detectOperation(gitDir)
	r.FetchedAt = fetchedAt(gitDir)
	r.HasRemote = hasRemote(ctx, runner, dir)
	r.Stashes = loadStashes(ctx, runner, dir)
	if r.HasRemote {
		r.Branches = loadBranches(ctx, runner, dir)
	}
	if r.Unborn {
		return r
	}
	r.LastCommit = lastCommit(ctx, runner, dir)
	r.loadUnpushed(ctx, runner, dir)
	return r
}

func (r *Repo) applyStatus(s status) {
	r.Files = s.files
	r.Unborn = s.oid == unbornOID
	r.Branch = s.head
	if s.head == detachedHead {
		r.Detached = true
		r.Branch = shortHash(s.oid)
	}
	r.Upstream = s.upstream
	r.UpstreamGone = s.upstream != "" && !s.hasAB
	r.Ahead, r.Behind = s.ahead, s.behind
}

func (r *Repo) loadUnpushed(ctx context.Context, runner macos.Runner, dir string) {
	if !r.HasRemote {
		return
	}
	r.UnpushedCommits = parseCommits(gitOutput(ctx, runner, dir, "log", "-n", strconv.Itoa(maxUnpushedListed), commitFormat, "HEAD", "--not", "--remotes"))
	if r.HasUpstream() {
		r.Unpushed = r.Ahead
		return
	}
	r.Unpushed, _ = strconv.Atoi(strings.TrimSpace(gitOutput(ctx, runner, dir, "rev-list", "--count", "HEAD", "--not", "--remotes")))
}

func hasRemote(ctx context.Context, runner macos.Runner, dir string) bool {
	return strings.TrimSpace(gitOutput(ctx, runner, dir, "remote")) != ""
}

func lastCommit(ctx context.Context, runner macos.Runner, dir string) Commit {
	commits := parseCommits(gitOutput(ctx, runner, dir, "log", "-n", "1", commitFormat))
	if len(commits) == 0 {
		return Commit{}
	}
	return commits[0]
}

func loadStashes(ctx context.Context, runner macos.Runner, dir string) []Stash {
	var stashes []Stash
	for _, fields := range records(gitOutput(ctx, runner, dir, "stash", "list", "--format=%gd%x00%ct%x00%gs"), 3) {
		stashes = append(stashes, Stash{Ref: fields[0], At: unixTime(fields[1]), Message: fields[2]})
	}
	return stashes
}

func parseCommits(out string) []Commit {
	var commits []Commit
	for _, fields := range records(out, 3) {
		commits = append(commits, Commit{Hash: fields[0], At: unixTime(fields[1]), Subject: fields[2]})
	}
	return commits
}

// records splits git output of one line per record, with fields separated by NUL bytes. Lines
// without the expected number of fields are skipped.
func records(out string, fields int) [][]string {
	var parsed [][]string
	for line := range strings.Lines(out) {
		parts := strings.SplitN(strings.TrimRight(line, "\n"), fieldSeparator, fields)
		if len(parts) == fields {
			parsed = append(parsed, parts)
		}
	}
	return parsed
}

func unixTime(s string) time.Time {
	seconds, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}

// git runs a git command in dir without taking locks, so reading never counts as a change.
func git(ctx context.Context, runner macos.Runner, dir string, args ...string) ([]byte, error) {
	return runner.Run(ctx, "git", append([]string{"--no-optional-locks", "-C", dir}, args...)...)
}

// gitOutput is git's output, or "" when the command fails: used for the details that are only
// nice to have once status has worked.
func gitOutput(ctx context.Context, runner macos.Runner, dir string, args ...string) string {
	out, err := git(ctx, runner, dir, args...)
	if err != nil {
		return ""
	}
	return string(out)
}
