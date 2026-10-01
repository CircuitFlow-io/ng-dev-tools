package cli

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls/tui"
)

var checkStateNames = map[pulls.CheckState]string{
	pulls.Failed:  "failed",
	pulls.Pending: "pending",
	pulls.Passed:  "passed",
	pulls.Skipped: "skipped",
}

type prsJSON struct {
	Viewer   string   `json:"viewer"`
	ToReview []prJSON `json:"toReview"`
	Mine     []prJSON `json:"mine"`
}

type prJSON struct {
	Repo   string `json:"repo"`
	Number int    `json:"number"`
	Title  string `json:"title"`
	URL    string `json:"url"`
	Author string `json:"author"`
	// State says what the pull request is waiting for, in the list's words.
	State          string          `json:"state"`
	Draft          bool            `json:"draft"`
	HeadRef        string          `json:"headRef"`
	BaseRef        string          `json:"baseRef"`
	CreatedAt      time.Time       `json:"createdAt,omitzero"`
	UpdatedAt      time.Time       `json:"updatedAt,omitzero"`
	Additions      int             `json:"additions"`
	Deletions      int             `json:"deletions"`
	ChangedFiles   int             `json:"changedFiles"`
	Comments       int             `json:"comments"`
	ReviewDecision string          `json:"reviewDecision,omitempty"`
	Mergeable      string          `json:"mergeable,omitempty"`
	CheckCounts    checkCountsJSON `json:"checkCounts"`
	Checks         []checkJSON     `json:"checks,omitempty"`
	Reviews        []prReviewJSON  `json:"reviews,omitempty"`
	Requested      []string        `json:"requested,omitempty"`
	Body           string          `json:"body,omitempty"`
}

type checkCountsJSON struct {
	Failed  int `json:"failed"`
	Pending int `json:"pending"`
	Passed  int `json:"passed"`
	Skipped int `json:"skipped"`
}

type checkJSON struct {
	Name     string `json:"name"`
	Workflow string `json:"workflow,omitempty"`
	State    string `json:"state"`
	URL      string `json:"url,omitempty"`
}

type prReviewJSON struct {
	Author string `json:"author"`
	State  string `json:"state"`
}

func toPRsJSON(d pulls.Dashboard) prsJSON {
	return prsJSON{
		Viewer:   d.Viewer,
		ToReview: nonNil(mapSlice(d.ToReview, toPRJSON)),
		Mine:     nonNil(mapSlice(d.Mine, toPRJSON)),
	}
}

func toPRJSON(p pulls.PR) prJSON {
	counts := p.CheckCounts()
	return prJSON{
		Repo:           p.Repo,
		Number:         p.Number,
		Title:          p.Title,
		URL:            p.URL,
		Author:         p.Author,
		State:          tui.StateText(p),
		Draft:          p.Draft,
		HeadRef:        p.HeadRef,
		BaseRef:        p.BaseRef,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
		Additions:      p.Additions,
		Deletions:      p.Deletions,
		ChangedFiles:   p.ChangedFiles,
		Comments:       p.Comments,
		ReviewDecision: p.ReviewDecision,
		Mergeable:      p.Mergeable,
		CheckCounts:    checkCountsJSON(counts),
		Checks:         mapSlice(p.Checks, toCheckJSON),
		Reviews:        mapSlice(p.Reviews, toPRReviewJSON),
		Requested:      p.Requested,
		Body:           p.Body,
	}
}

func toCheckJSON(c pulls.Check) checkJSON {
	return checkJSON{Name: c.Name, Workflow: c.Workflow, State: checkStateNames[c.State], URL: c.URL}
}

func toPRReviewJSON(r pulls.Review) prReviewJSON {
	return prReviewJSON{Author: r.Author, State: r.State}
}
