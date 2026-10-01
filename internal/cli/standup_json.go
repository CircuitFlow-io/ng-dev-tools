package cli

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/standup"
)

type standupJSON struct {
	Since time.Time `json:"since"`
	// LastWorkedDay is set when since is the last day you committed, rather than --since.
	LastWorkedDay bool                  `json:"lastWorkedDay"`
	Projects      []standupProjectJSON  `json:"projects"`
	Reviewed      []standupReviewedJSON `json:"reviewed"`
	InProgress    []repoJSON            `json:"inProgress"`
	GitHubError   string                `json:"githubError,omitempty"`
}

type standupProjectJSON struct {
	Name   string             `json:"name"`
	Dir    string             `json:"dir,omitempty"`
	URL    string             `json:"url,omitempty"`
	Groups []standupGroupJSON `json:"groups"`
}

// standupGroupJSON is the commits of one pull request or branch. Branch is empty for commits made
// straight on the default branch.
type standupGroupJSON struct {
	PR            *standupPRJSON      `json:"pr,omitempty"`
	Branch        string              `json:"branch,omitempty"`
	DefaultBranch string              `json:"defaultBranch,omitempty"`
	Commits       []standupCommitJSON `json:"commits,omitempty"`
}

type standupPRJSON struct {
	Repo      string    `json:"repo"`
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	URL       string    `json:"url"`
	Branch    string    `json:"branch"`
	State     string    `json:"state"`
	Draft     bool      `json:"draft"`
	CreatedAt time.Time `json:"createdAt,omitzero"`
	MergedAt  time.Time `json:"mergedAt,omitzero"`
	ClosedAt  time.Time `json:"closedAt,omitzero"`
}

type standupCommitJSON struct {
	Hash    string    `json:"hash"`
	At      time.Time `json:"at"`
	Subject string    `json:"subject"`
	URL     string    `json:"url,omitempty"`
}

type standupReviewedJSON struct {
	Repo    string    `json:"repo"`
	Number  int       `json:"number"`
	Title   string    `json:"title"`
	URL     string    `json:"url"`
	Author  string    `json:"author"`
	State   string    `json:"state"`
	Verdict string    `json:"verdict"`
	At      time.Time `json:"at,omitzero"`
}

func toStandupJSON(r standup.Report) standupJSON {
	return standupJSON{
		Since:         r.Since,
		LastWorkedDay: r.LastWorkedDay,
		Projects:      nonNil(mapSlice(r.Projects, toStandupProjectJSON)),
		Reviewed:      nonNil(mapSlice(r.Reviewed, toStandupReviewedJSON)),
		InProgress:    nonNil(mapSlice(r.InProgress, toRepoJSON)),
		GitHubError:   errText(r.GitHubErr),
	}
}

func toStandupProjectJSON(p standup.Project) standupProjectJSON {
	return standupProjectJSON{Name: p.Name, Dir: p.Dir, URL: p.URL, Groups: mapSlice(p.Groups, toStandupGroupJSON)}
}

func toStandupGroupJSON(g standup.Group) standupGroupJSON {
	view := standupGroupJSON{Branch: g.Branch, DefaultBranch: g.DefaultBranch, Commits: mapSlice(g.Commits, toStandupCommitJSON)}
	if g.PR != nil {
		pr := toStandupPRJSON(*g.PR)
		view.PR = &pr
	}
	return view
}

func toStandupPRJSON(p standup.PR) standupPRJSON {
	return standupPRJSON{
		Repo:      p.Repo,
		Number:    p.Number,
		Title:     p.Title,
		URL:       p.URL,
		Branch:    p.Branch,
		State:     p.State,
		Draft:     p.Draft,
		CreatedAt: p.CreatedAt,
		MergedAt:  p.MergedAt,
		ClosedAt:  p.ClosedAt,
	}
}

func toStandupCommitJSON(c standup.Commit) standupCommitJSON {
	return standupCommitJSON{Hash: c.Hash, At: c.At, Subject: c.Subject, URL: c.URL}
}

func toStandupReviewedJSON(r standup.Reviewed) standupReviewedJSON {
	return standupReviewedJSON{
		Repo:    r.Repo,
		Number:  r.Number,
		Title:   r.Title,
		URL:     r.URL,
		Author:  r.Author,
		State:   r.State,
		Verdict: r.Verdict,
		At:      r.At,
	}
}
