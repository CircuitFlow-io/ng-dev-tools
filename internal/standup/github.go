package standup

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
)

const ghLoginHint = "gh auth login"

// Review verdicts, as GitHub names review states.
const (
	Approved         = "APPROVED"
	ChangesRequested = "CHANGES_REQUESTED"
	Commented        = "COMMENTED"
)

// PR states, as GitHub names them.
const (
	Open   = "OPEN"
	Merged = "MERGED"
	Closed = "CLOSED"
)

// activityQuery finds, in one request, your pull requests updated since the start and the others'
// ones you reviewed or commented on. Search is the only way GitHub offers to find the latter.
const activityQuery = `query($authored: String!, $reviewed: String!, $commented: String!) {
  viewer { login }
  authored: search(query: $authored, type: ISSUE, first: 50) { nodes { ... on PullRequest {
    number title url state isDraft createdAt mergedAt closedAt headRefName repository { nameWithOwner }
    commits(first: 100) { totalCount nodes { commit { oid } } }
  } } }
  reviewed: search(query: $reviewed, type: ISSUE, first: 50) { nodes { ...others } }
  commented: search(query: $commented, type: ISSUE, first: 50) { nodes { ...others } }
}

fragment others on PullRequest {
  number title url state repository { nameWithOwner } author { login }
  reviews(last: 50) { nodes { author { login } state submittedAt } }
  comments(last: 50) { nodes { author { login } createdAt } }
}`

// PR is one of your pull requests.
type PR struct {
	// Repo is "owner/name".
	Repo      string
	Number    int
	Title     string
	URL       string
	Branch    string
	State     string
	Draft     bool
	CreatedAt time.Time
	MergedAt  time.Time
	ClosedAt  time.Time
	// commits are the hashes of the pull request's commits, and commitCount how many it has.
	commits     map[string]bool
	commitCount int
}

// Ref is "owner/name#number".
func (p PR) Ref() string {
	return p.Repo + "#" + strconv.Itoa(p.Number)
}

// Reviewed is someone else's pull request you reviewed or commented on.
type Reviewed struct {
	Repo   string
	Number int
	Title  string
	URL    string
	Author string
	State  string
	// Verdict is your latest verdict since the start: Approved, ChangesRequested or Commented.
	Verdict string
	At      time.Time
}

// Ref is "owner/name#number".
func (r Reviewed) Ref() string {
	return r.Repo + "#" + strconv.Itoa(r.Number)
}

// github is what you did on GitHub since the start of the report.
type github struct {
	viewer   string
	authored []PR
	reviewed []Reviewed
}

type activityResponse struct {
	Data struct {
		Viewer struct {
			Login string `json:"login"`
		} `json:"viewer"`
		Authored struct {
			Nodes []authoredNode `json:"nodes"`
		} `json:"authored"`
		Reviewed struct {
			Nodes []othersNode `json:"nodes"`
		} `json:"reviewed"`
		Commented struct {
			Nodes []othersNode `json:"nodes"`
		} `json:"commented"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type repository struct {
	NameWithOwner string `json:"nameWithOwner"`
}

type login struct {
	Login string `json:"login"`
}

type authoredNode struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	URL         string     `json:"url"`
	State       string     `json:"state"`
	IsDraft     bool       `json:"isDraft"`
	CreatedAt   time.Time  `json:"createdAt"`
	MergedAt    *time.Time `json:"mergedAt"`
	ClosedAt    *time.Time `json:"closedAt"`
	HeadRefName string     `json:"headRefName"`
	Repository  repository `json:"repository"`
	Commits     struct {
		TotalCount int `json:"totalCount"`
		Nodes      []struct {
			Commit struct {
				OID string `json:"oid"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type othersNode struct {
	Number     int        `json:"number"`
	Title      string     `json:"title"`
	URL        string     `json:"url"`
	State      string     `json:"state"`
	Repository repository `json:"repository"`
	Author     *login     `json:"author"`
	Reviews    struct {
		Nodes []struct {
			Author      *login    `json:"author"`
			State       string    `json:"state"`
			SubmittedAt time.Time `json:"submittedAt"`
		} `json:"nodes"`
	} `json:"reviews"`
	Comments struct {
		Nodes []struct {
			Author    *login    `json:"author"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"nodes"`
	} `json:"comments"`
}

// loadGitHub reads your pull request activity since since through gh, which holds the GitHub login.
func loadGitHub(ctx context.Context, runner macos.Runner, since time.Time) (github, error) {
	if !runner.Available("gh") {
		return github{}, pulls.ErrNoGH
	}
	updated := " updated:>=" + since.UTC().Format(time.RFC3339)
	out, err := runner.Run(ctx, "gh", "api", "graphql",
		"-f", "authored=is:pr author:@me"+updated,
		"-f", "reviewed=is:pr reviewed-by:@me -author:@me"+updated,
		"-f", "commented=is:pr commenter:@me -author:@me"+updated,
		"-f", "query="+activityQuery)
	if err != nil {
		if strings.Contains(err.Error(), ghLoginHint) {
			return github{}, pulls.ErrNotLoggedIn
		}
		return github{}, err
	}
	return parseActivity(out, since)
}

func parseActivity(data []byte, since time.Time) (github, error) {
	var resp activityResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return github{}, err
	}
	if len(resp.Errors) > 0 {
		return github{}, errors.New(resp.Errors[0].Message)
	}
	g := github{viewer: resp.Data.Viewer.Login}
	for _, node := range resp.Data.Authored.Nodes {
		if node.Number > 0 {
			g.authored = append(g.authored, node.pr())
		}
	}
	seen := map[string]bool{}
	for _, node := range slices.Concat(resp.Data.Reviewed.Nodes, resp.Data.Commented.Nodes) {
		r, ok := node.reviewed(g.viewer, since)
		if ok && !seen[r.Ref()] {
			seen[r.Ref()] = true
			g.reviewed = append(g.reviewed, r)
		}
	}
	slices.SortFunc(g.reviewed, func(a, b Reviewed) int { return cmp.Or(b.At.Compare(a.At), cmp.Compare(a.Ref(), b.Ref())) })
	return g, nil
}

func (n authoredNode) pr() PR {
	p := PR{
		Repo:        n.Repository.NameWithOwner,
		Number:      n.Number,
		Title:       n.Title,
		URL:         n.URL,
		Branch:      n.HeadRefName,
		State:       n.State,
		Draft:       n.IsDraft,
		CreatedAt:   n.CreatedAt,
		commits:     map[string]bool{},
		commitCount: n.Commits.TotalCount,
	}
	if n.MergedAt != nil {
		p.MergedAt = *n.MergedAt
	}
	if n.ClosedAt != nil {
		p.ClosedAt = *n.ClosedAt
	}
	for _, c := range n.Commits.Nodes {
		p.commits[c.Commit.OID] = true
	}
	return p
}

// reviewed is your part in someone else's pull request since since: your reviews and comments
// in that time. It is false when you left neither.
func (n othersNode) reviewed(viewer string, since time.Time) (Reviewed, bool) {
	r := Reviewed{
		Repo:   n.Repository.NameWithOwner,
		Number: n.Number,
		Title:  n.Title,
		URL:    n.URL,
		State:  n.State,
		Author: loginOf(n.Author),
	}
	for _, review := range n.Reviews.Nodes {
		if loginOf(review.Author) == viewer && !review.SubmittedAt.Before(since) {
			r.note(review.State, review.SubmittedAt)
		}
	}
	for _, comment := range n.Comments.Nodes {
		if loginOf(comment.Author) == viewer && !comment.CreatedAt.Before(since) {
			r.note(Commented, comment.CreatedAt)
		}
	}
	return r, r.Verdict != ""
}

// note records a review or a comment. An approval or a request for changes outweighs a comment,
// and otherwise the newer one wins.
func (r *Reviewed) note(state string, at time.Time) {
	verdict := state
	if verdict != Approved && verdict != ChangesRequested {
		verdict = Commented
	}
	if r.Verdict == "" || isDecision(verdict) && !isDecision(r.Verdict) || isDecision(verdict) == isDecision(r.Verdict) && at.After(r.At) {
		r.Verdict = verdict
	}
	if at.After(r.At) {
		r.At = at
	}
}

func isDecision(verdict string) bool {
	return verdict != Commented
}

// loginOf names a GitHub account, or "ghost" for a deleted one, as GitHub itself does.
func loginOf(l *login) string {
	if l == nil {
		return "ghost"
	}
	return l.Login
}
