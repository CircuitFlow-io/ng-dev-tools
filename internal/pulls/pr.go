// Package pulls reads the open pull requests that involve you on GitHub (the ones you opened and
// the ones waiting for your review) through the gh CLI, and acts on them locally.
package pulls

import (
	"strconv"
	"time"
)

// Review decisions and review states as GitHub names them.
const (
	Approved         = "APPROVED"
	ChangesRequested = "CHANGES_REQUESTED"
	ReviewRequired   = "REVIEW_REQUIRED"
	conflicting      = "CONFLICTING"
	mergeable        = "MERGEABLE"
)

// PR is one open pull request.
type PR struct {
	// Repo is "owner/name".
	Repo         string
	Number       int
	Title        string
	URL          string
	Author       string
	Body         string
	Draft        bool
	HeadRef      string
	BaseRef      string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	Additions    int
	Deletions    int
	ChangedFiles int
	Comments     int
	// ReviewDecision is Approved, ChangesRequested, ReviewRequired or "" when the repository asks
	// for no review.
	ReviewDecision string
	// Mergeable is GitHub's MERGEABLE, CONFLICTING or UNKNOWN while it is still computing.
	Mergeable string
	Checks    []Check
	// Reviews holds each reviewer's latest review.
	Reviews []Review
	// Requested are the users and teams asked to review who have not yet.
	Requested []string
}

// Review is a reviewer's latest verdict: Approved, ChangesRequested or COMMENTED.
type Review struct {
	Author string
	State  string
}

// Ref names the pull request as "owner/name#123".
func (p PR) Ref() string {
	return p.Repo + "#" + strconv.Itoa(p.Number)
}

// Conflicting reports whether the branch conflicts with its base.
func (p PR) Conflicting() bool {
	return p.Mergeable == conflicting
}

// NoConflicts reports whether GitHub has confirmed the branch merges cleanly into its base.
func (p PR) NoConflicts() bool {
	return p.Mergeable == mergeable
}

// Status is the most important thing to know about a pull request, most urgent first.
type Status int

const (
	Conflicts Status = iota
	ChecksFailing
	NeedsChanges
	Draft
	ChecksRunning
	CheckingConflicts
	Ready
	Waiting
)

// Status sums up what the pull request is waiting for.
func (p PR) Status() Status {
	checks := p.CheckCounts()
	switch {
	case p.Conflicting():
		return Conflicts
	case checks.Failed > 0:
		return ChecksFailing
	case p.ReviewDecision == ChangesRequested:
		return NeedsChanges
	case p.Draft:
		return Draft
	case checks.Pending > 0:
		return ChecksRunning
	case p.ReviewDecision == ReviewRequired:
		return Waiting
	case !p.NoConflicts():
		return CheckingConflicts
	default:
		return Ready
	}
}
