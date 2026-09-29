// Package standup gathers what you did since a given day across your projects: the commits you
// wrote, by project and by branch or pull request, the pull requests you opened, merged or
// reviewed on GitHub, and the work you have in progress now. It only reads.
package standup

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
)

// Report is what you did since Since.
type Report struct {
	Since time.Time
	// LastWorkedDay is set when Since was worked out as the last day you committed.
	LastWorkedDay bool
	Projects      []Project
	Reviewed      []Reviewed
	// InProgress are the repositories with uncommitted changes or unpushed commits now.
	InProgress []gitstatus.Repo
	// GitHubErr is why GitHub could not be read; the report then only has local work.
	GitHubErr error
}

// Project is your work in one repository.
type Project struct {
	Name string
	// Dir is the local clone, empty for a repository you only have on GitHub.
	Dir string
	// URL is the repository on GitHub, empty when it is not there.
	URL    string
	Groups []Group
}

// Group is the commits of one pull request or branch.
type Group struct {
	// PR is the pull request the commits belong to; nil for a branch without one.
	PR *PR
	// Branch is the branch name, "" for commits straight on the default branch.
	Branch        string
	DefaultBranch string
	Commits       []Commit
}

// Commits counts the commits in the report.
func (r Report) Commits() int {
	n := 0
	for _, p := range r.Projects {
		for _, g := range p.Groups {
			n += len(g.Commits)
		}
	}
	return n
}

// PRs counts your pull requests by what happened to them since the start: opened and merged.
func (r Report) PRs() (opened, merged int) {
	for _, p := range r.Projects {
		for _, g := range p.Groups {
			if g.PR == nil {
				continue
			}
			if g.PR.OpenedSince(r.Since) {
				opened++
			}
			if g.PR.MergedSince(r.Since) {
				merged++
			}
		}
	}
	return opened, merged
}

// Empty reports whether there is nothing to tell.
func (r Report) Empty() bool {
	return len(r.Projects) == 0 && len(r.Reviewed) == 0 && len(r.InProgress) == 0
}

// OpenedSince reports whether the pull request was opened at or after since.
func (p PR) OpenedSince(since time.Time) bool {
	return !p.CreatedAt.Before(since)
}

// MergedSince reports whether the pull request was merged at or after since.
func (p PR) MergedSince(since time.Time) bool {
	return !p.MergedAt.IsZero() && !p.MergedAt.Before(since)
}

// ClosedSince reports whether the pull request was closed without merging at or after since.
func (p PR) ClosedSince(since time.Time) bool {
	return p.State == Closed && !p.ClosedAt.Before(since)
}

func (p PR) touchedSince(since time.Time) bool {
	return p.OpenedSince(since) || p.MergedSince(since) || p.ClosedSince(since)
}

// latest is when the group last saw activity since since, for ordering.
func (g Group) latest(since time.Time) time.Time {
	var t time.Time
	if len(g.Commits) > 0 {
		t = g.Commits[0].At
	}
	if g.PR == nil {
		return t
	}
	for _, event := range []time.Time{g.PR.CreatedAt, g.PR.MergedAt, g.PR.ClosedAt} {
		if !event.Before(since) && event.After(t) {
			t = event
		}
	}
	return t
}

// build puts the local commits under their pull request or branch, adds the pull requests with
// no commits here that were opened, merged or closed since the start, and orders everything
// newest first.
func build(since time.Time, work []localWork, prs []PR) []Project {
	byRepo := map[string][]*PR{}
	for i := range prs {
		repo := strings.ToLower(prs[i].Repo)
		byRepo[repo] = append(byRepo[repo], &prs[i])
	}
	var projects []Project
	placed := map[*PR]bool{}
	shown := map[string]bool{}
	for _, w := range work {
		repoPRs := byRepo[w.repo.github]
		p := Project{Name: w.repo.name, Dir: w.repo.dir, URL: repoURL(w.repo.github), Groups: w.groups(repoPRs, shown)}
		for _, pr := range repoPRs {
			if pr.touchedSince(since) && !placed[pr] && !hasGroup(p.Groups, pr) {
				p.Groups = append(p.Groups, Group{PR: pr, Branch: pr.Branch})
			}
		}
		for _, pr := range repoPRs {
			placed[pr] = true
		}
		if len(p.Groups) > 0 {
			projects = append(projects, p)
		}
	}
	projects = append(projects, remoteOnly(since, prs, placed)...)
	for i := range projects {
		slices.SortStableFunc(projects[i].Groups, func(a, b Group) int { return b.latest(since).Compare(a.latest(since)) })
	}
	slices.SortStableFunc(projects, func(a, b Project) int {
		return cmp.Or(b.Groups[0].latest(since).Compare(a.Groups[0].latest(since)), cmp.Compare(a.Name, b.Name))
	})
	return projects
}

// groups puts each commit under the pull request that has it, then under the pull request for its
// branch, then under its branch. Commits already shown, by another clone of the same repository,
// are left out and marked in shown.
func (w localWork) groups(prs []*PR, shown map[string]bool) []Group {
	var groups []Group
	index := map[string]int{}
	for _, c := range w.commits {
		if shown[c.Hash] {
			continue
		}
		shown[c.Hash] = true
		pr := prWithCommit(prs, c.Hash)
		branch := w.branchOf[c.Hash]
		if pr == nil && branch != "" {
			pr = prForBranch(prs, branch)
		}
		key := "branch " + branch
		if pr != nil {
			key, branch = "pr "+pr.Ref(), pr.Branch
		}
		i, ok := index[key]
		if !ok {
			i = len(groups)
			index[key] = i
			groups = append(groups, Group{PR: pr, Branch: branch, DefaultBranch: w.repo.defaultBranch})
		}
		groups[i].Commits = append(groups[i].Commits, c)
	}
	return groups
}

// prWithCommit is the pull request holding the commit, the smallest when several do: a pull
// request based on another one holds that one's commits too.
func prWithCommit(prs []*PR, hash string) *PR {
	var found *PR
	for _, pr := range prs {
		if pr.commits[hash] && (found == nil || pr.commitCount < found.commitCount) {
			found = pr
		}
	}
	return found
}

// prForBranch is the open pull request for branch, for commits not pushed to it yet.
func prForBranch(prs []*PR, branch string) *PR {
	for _, pr := range prs {
		if pr.Branch == branch && pr.State == Open {
			return pr
		}
	}
	return nil
}

func hasGroup(groups []Group, pr *PR) bool {
	return slices.ContainsFunc(groups, func(g Group) bool { return g.PR == pr })
}

// remoteOnly makes a project, named after the repository, for each repository with pull requests
// opened, merged or closed since the start that has no local clone.
func remoteOnly(since time.Time, prs []PR, placed map[*PR]bool) []Project {
	var projects []Project
	index := map[string]int{}
	for i := range prs {
		pr := &prs[i]
		if placed[pr] || !pr.touchedSince(since) {
			continue
		}
		at, ok := index[pr.Repo]
		if !ok {
			at = len(projects)
			index[pr.Repo] = at
			projects = append(projects, Project{Name: pr.Repo, URL: repoURL(pr.Repo)})
		}
		projects[at].Groups = append(projects[at].Groups, Group{PR: pr, Branch: pr.Branch})
	}
	return projects
}

// inProgress keeps the repositories with uncommitted changes or commits not pushed yet.
func inProgress(repos []gitstatus.Repo) []gitstatus.Repo {
	return slices.DeleteFunc(repos, func(r gitstatus.Repo) bool {
		return r.Err != nil || !r.Changes().Any() && r.Unpushed == 0 && !r.NotPushed()
	})
}

func repoURL(repo string) string {
	if repo == "" {
		return ""
	}
	return githubURL + repo
}
