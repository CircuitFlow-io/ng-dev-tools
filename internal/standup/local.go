package standup

import (
	"cmp"
	"context"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
)

const (
	fieldSeparator  = "\x00"
	commitFormat    = "--format=%H%x00%at%x00%s"
	refFormat       = "--format=%(refname)%00%(committerdate:unix)"
	headsPrefix     = "refs/heads/"
	remotesPrefix   = "refs/remotes/"
	remoteHead      = "/HEAD"
	githubURL       = "https://github.com/"
	shortHashLength = 7
	// lookbackDays is how far back the last day you worked is looked for.
	lookbackDays = 30
)

// fallbackDefaults are the default branch names tried when origin/HEAD is not set.
var fallbackDefaults = []string{"main", "master"}

// Commit is one commit you wrote.
type Commit struct {
	Hash    string
	At      time.Time
	Subject string
	// URL is the commit on GitHub, empty when it is not pushed or the repository is not on GitHub.
	URL string
}

// Short is the abbreviated hash.
func (c Commit) Short() string {
	return c.Hash[:min(len(c.Hash), shortHashLength)]
}

// repo is one local repository and how to find your commits in it.
type repo struct {
	name string
	dir  string
	// email is your user.email there; without one, no commits are yours.
	email string
	// github is "owner/name" in lower case, empty without a GitHub remote.
	github string
	// defaultBranch is the short name of the default branch, and defaultRef what to compare with.
	defaultBranch string
	defaultRef    string
}

// localWork is what you committed in one repository since the start of the report.
type localWork struct {
	repo    repo
	commits []Commit
	// branchOf names the branch each commit belongs to, "" for the default branch.
	branchOf map[string]string
}

func readRepo(ctx context.Context, runner macos.Runner, name, dir string) repo {
	r := repo{name: name, dir: dir}
	r.email = strings.TrimSpace(gitOutput(ctx, runner, dir, "config", "user.email"))
	r.github, _ = pulls.RepoFromURL(gitOutput(ctx, runner, dir, "remote", "get-url", "origin"))
	r.defaultBranch, r.defaultRef = defaultBranch(ctx, runner, dir)
	return r
}

// defaultBranch finds the branch work is merged into: origin/HEAD, or else a local main or master.
func defaultBranch(ctx context.Context, runner macos.Runner, dir string) (name, ref string) {
	if ref := strings.TrimSpace(gitOutput(ctx, runner, dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")); ref != "" {
		_, name, _ := strings.Cut(ref, "/")
		return name, ref
	}
	for _, name := range fallbackDefaults {
		if _, err := git(ctx, runner, dir, "rev-parse", "--verify", "--quiet", headsPrefix+name); err == nil {
			return name, name
		}
	}
	return "", ""
}

// yourCommits lists the commits you wrote on any local or remote branch since since, newest
// first. Git filters on the commit date; the author date is what counts, so a rebase does not
// bring old work back.
func (r repo) yourCommits(ctx context.Context, runner macos.Runner, since time.Time) []Commit {
	if r.email == "" {
		return nil
	}
	out := gitOutput(ctx, runner, r.dir, "log", "--branches", "--remotes", "--no-merges", "--regexp-ignore-case",
		"--author="+regexp.QuoteMeta(r.email), "--since="+since.Format(time.RFC3339), commitFormat)
	var commits []Commit
	for _, fields := range records(out, 3) {
		c := Commit{Hash: fields[0], At: unixTime(fields[1]), Subject: fields[2]}
		if !c.At.Before(since) {
			commits = append(commits, c)
		}
	}
	return commits
}

func (r repo) load(ctx context.Context, runner macos.Runner, since time.Time) localWork {
	work := localWork{repo: r, commits: r.yourCommits(ctx, runner, since)}
	if len(work.commits) == 0 {
		return work
	}
	work.branchOf = r.branchesOf(ctx, runner, work.commits, since)
	unpushed := r.unpushed(ctx, runner)
	for i, c := range work.commits {
		if r.github != "" && !unpushed[c.Hash] {
			work.commits[i].URL = repoURL(r.github) + "/commit/" + c.Hash
		}
	}
	return work
}

// unpushed is the set of commits on local branches that no remote branch has.
func (r repo) unpushed(ctx context.Context, runner macos.Runner) map[string]bool {
	set := map[string]bool{}
	for _, hash := range strings.Fields(gitOutput(ctx, runner, r.dir, "rev-list", "--branches", "--not", "--remotes")) {
		set[hash] = true
	}
	return set
}

// branchesOf names the branch each commit was made on: the feature branch holding it that is not
// merged into the default branch, the smallest when several do (a branch started from another
// holds that one's commits too), or "" for the default branch.
func (r repo) branchesOf(ctx context.Context, runner macos.Runner, commits []Commit, since time.Time) map[string]string {
	wanted := map[string]bool{}
	for _, c := range commits {
		wanted[c.Hash] = true
	}
	type branch struct {
		name   string
		hashes []string
	}
	var branches []branch
	for name, refs := range r.recentBranches(ctx, runner, since) {
		args := append([]string{"log", "--no-merges", "--format=%H", "--since=" + since.Format(time.RFC3339)}, refs...)
		if r.defaultRef != "" {
			args = append(args, "--not", r.defaultRef)
		}
		b := branch{name: name}
		for _, hash := range strings.Fields(gitOutput(ctx, runner, r.dir, args...)) {
			if wanted[hash] {
				b.hashes = append(b.hashes, hash)
			}
		}
		branches = append(branches, b)
	}
	slices.SortFunc(branches, func(a, b branch) int {
		return cmp.Or(cmp.Compare(len(a.hashes), len(b.hashes)), cmp.Compare(a.name, b.name))
	})
	branchOf := map[string]string{}
	for _, b := range branches {
		for _, hash := range b.hashes {
			if _, taken := branchOf[hash]; !taken {
				branchOf[hash] = b.name
			}
		}
	}
	return branchOf
}

// recentBranches maps each branch name other than the default to its local and remote refs, for
// the branches with a commit since since. A branch's tip is its newest commit, so older ones
// cannot hold any of the report's commits.
func (r repo) recentBranches(ctx context.Context, runner macos.Runner, since time.Time) map[string][]string {
	branches := map[string][]string{}
	for _, fields := range records(gitOutput(ctx, runner, r.dir, "for-each-ref", refFormat, headsPrefix, remotesPrefix), 2) {
		ref, tip := fields[0], unixTime(fields[1])
		name, ok := branchName(ref)
		if !ok || name == r.defaultBranch || tip.Before(since) {
			continue
		}
		branches[name] = append(branches[name], ref)
	}
	return branches
}

// branchName is the branch a local or remote ref stands for: refs/remotes/origin/feat/x is feat/x.
func branchName(ref string) (string, bool) {
	if name, ok := strings.CutPrefix(ref, headsPrefix); ok {
		return name, true
	}
	rest, ok := strings.CutPrefix(ref, remotesPrefix)
	if !ok || strings.HasSuffix(rest, remoteHead) {
		return "", false
	}
	_, name, ok := strings.Cut(rest, "/")
	return name, ok
}

// commitTimes is when you wrote each commit in the lookback before now, for finding the last day
// you worked.
func (r repo) commitTimes(ctx context.Context, runner macos.Runner, now time.Time) []time.Time {
	var times []time.Time
	for _, c := range r.yourCommits(ctx, runner, StartOfDay(now).AddDate(0, 0, -lookbackDays)) {
		times = append(times, c.At)
	}
	return times
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

// gitOutput is git's output, or "" when the command fails.
func gitOutput(ctx context.Context, runner macos.Runner, dir string, args ...string) string {
	out, err := git(ctx, runner, dir, args...)
	if err != nil {
		return ""
	}
	return string(out)
}
