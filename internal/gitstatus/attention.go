package gitstatus

import (
	"cmp"
	"slices"
)

// Level is how much a repository needs looking at, most urgent first.
type Level int

const (
	// Conflict is an operation in progress, conflicted files or a repository git could not read.
	Conflict Level = iota
	// Behind is a checked-out branch missing commits from its upstream.
	Behind
	// Uncommitted is a working tree with changes.
	Uncommitted
	// Unpushed is a checked-out branch with commits on no remote, or never pushed.
	Unpushed
	// Stale is stashes or other branches worth a look, and nothing more urgent.
	Stale
	Clean
)

// NeedsAttention reports whether the level asks for something to be done, rather than being
// clean or only informational.
func (l Level) NeedsAttention() bool {
	return l < Stale
}

// Attention is r's most urgent level.
func (r Repo) Attention() Level {
	changes := r.Changes()
	switch {
	case r.Err != nil, r.Operation != NoOperation, changes.Conflicted > 0:
		return Conflict
	case r.Behind > 0:
		return Behind
	case changes.Any():
		return Uncommitted
	case r.Unpushed > 0, r.NotPushed():
		return Unpushed
	case len(r.Stashes) > 0, len(r.Branches) > 0, r.UpstreamGone:
		return Stale
	default:
		return Clean
	}
}

// NotPushed reports whether the checked-out branch has never been pushed to the remote.
func (r Repo) NotPushed() bool {
	return r.HasRemote && !r.Unborn && !r.Detached && r.Upstream == ""
}

// Sort orders repos most urgent first, then by the newest commit, then by name.
func Sort(repos []Repo) {
	slices.SortStableFunc(repos, func(a, b Repo) int {
		return cmp.Or(
			cmp.Compare(a.Attention(), b.Attention()),
			b.LastCommit.At.Compare(a.LastCommit.At),
			cmp.Compare(a.Name, b.Name),
		)
	})
}
