package gitstatus

import (
	"context"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	currentBranchMarker = "*"
	trackGone           = "gone"
	branchFormat        = "--format=%(HEAD)%00%(refname:short)%00%(upstream:short)%00%(upstream:track,nobracket)%00%(committerdate:unix)"
	branchFields        = 5
)

// Branch is a local branch other than the checked-out one.
type Branch struct {
	Name     string
	Upstream string
	Ahead    int
	Behind   int
	// Gone is an upstream deleted on the remote.
	Gone bool
	At   time.Time
}

// NotPushed reports whether the branch has never been pushed.
func (b Branch) NotPushed() bool {
	return b.Upstream == ""
}

func (b Branch) noteworthy() bool {
	return b.NotPushed() || b.Gone || b.Ahead > 0 || b.Behind > 0
}

func loadBranches(ctx context.Context, runner macos.Runner, dir string) []Branch {
	return parseBranches(gitOutput(ctx, runner, dir, "for-each-ref", branchFormat, "refs/heads"))
}

// parseBranches keeps the noteworthy branches other than the current one, most recent first.
func parseBranches(out string) []Branch {
	var branches []Branch
	for _, fields := range records(out, branchFields) {
		if fields[0] == currentBranchMarker {
			continue
		}
		b := Branch{Name: fields[1], Upstream: fields[2], At: unixTime(fields[4])}
		b.applyTrack(fields[3])
		if b.noteworthy() {
			branches = append(branches, b)
		}
	}
	slices.SortStableFunc(branches, func(a, b Branch) int { return b.At.Compare(a.At) })
	return branches
}

// applyTrack reads git's tracking summary: "ahead 3, behind 1", "behind 2", "gone" or "".
func (b *Branch) applyTrack(track string) {
	for part := range strings.SplitSeq(track, ", ") {
		word, count, _ := strings.Cut(part, " ")
		n, _ := strconv.Atoi(count)
		switch word {
		case "ahead":
			b.Ahead = n
		case "behind":
			b.Behind = n
		case trackGone:
			b.Gone = true
		}
	}
}

// BranchCounts summarises the noteworthy branches for a one-line note.
type BranchCounts struct {
	Behind    int
	Ahead     int
	NotPushed int
	Gone      int
}

// BranchCounts counts r's other branches by what makes them worth a look.
func (r Repo) BranchCounts() BranchCounts {
	var c BranchCounts
	for _, b := range r.Branches {
		c.Behind += boolCount(b.Behind > 0)
		c.Ahead += boolCount(b.Ahead > 0)
		c.NotPushed += boolCount(b.NotPushed())
		c.Gone += boolCount(b.Gone)
	}
	return c
}

func boolCount(b bool) int {
	if b {
		return 1
	}
	return 0
}
