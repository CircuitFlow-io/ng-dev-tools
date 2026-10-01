package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	aheadArrow  = "⇡"
	behindArrow = "⇣"
	inSync      = "✓"
	noteJoiner  = " · "
)

var (
	accent = lipgloss.NewStyle().Foreground(ui.ColorAccent)
	plain  = lipgloss.NewStyle()
)

type glyph struct {
	symbol string
	style  lipgloss.Style
}

var glyphs = map[gitstatus.Level]glyph{
	gitstatus.Conflict:    {"✖", ui.Danger},
	gitstatus.Behind:      {behindArrow, ui.Warning},
	gitstatus.Uncommitted: {"●", ui.Warning},
	gitstatus.Unpushed:    {aheadArrow, accent},
	gitstatus.Stale:       {"◦", ui.Muted},
	gitstatus.Clean:       {inSync, ui.Success},
}

func glyphCell(r gitstatus.Repo) ui.Cell {
	g := glyphs[r.Attention()]
	return ui.Cell{ui.NewSpan(g.symbol, g.style)}
}

func changesCell(r gitstatus.Repo) ui.Cell {
	if r.Err != nil {
		return ui.Cell{ui.NewSpan("unreadable", ui.Danger)}
	}
	c := r.Changes()
	if !c.Any() {
		return ui.Cell{ui.NewSpan("clean", ui.Success)}
	}
	var parts []ui.Cell
	for _, count := range []struct {
		prefix string
		n      int
		style  lipgloss.Style
	}{
		{"!", c.Conflicted, ui.Danger},
		{"+", c.Staged, ui.Success},
		{"~", c.Modified, ui.Warning},
		{"?", c.Untracked, ui.Muted},
	} {
		if count.n > 0 {
			parts = append(parts, ui.Cell{ui.NewSpan(count.prefix+strconv.Itoa(count.n), count.style)})
		}
	}
	return ui.JoinCells(parts, " ")
}

// syncState is what the table knows about a fetch of the repository.
type syncState struct {
	fetching bool
	spinner  string
	fetchErr string
}

func syncCell(r gitstatus.Repo, s syncState) ui.Cell {
	switch {
	case s.fetching:
		return ui.Cell{ui.NewSpan(s.spinner, ui.Title), ui.NewSpan(" fetching", ui.Muted)}
	case s.fetchErr != "":
		return ui.Cell{ui.NewSpan("fetch failed", ui.Danger)}
	case r.Err != nil:
		return nil
	case !r.HasRemote:
		return ui.Cell{ui.NewSpan("no remote", ui.Muted)}
	case r.Unborn:
		return ui.Cell{ui.NewSpan("no commits", ui.Muted)}
	case r.Detached:
		return ui.Cell{ui.NewSpan("detached", ui.Muted)}
	case r.UpstreamGone:
		return withUnpushed(r, ui.Cell{ui.NewSpan("remote gone", ui.Warning)})
	case r.NotPushed():
		return withUnpushed(r, ui.Cell{ui.NewSpan("not pushed", accent)})
	}
	return aheadBehind(r.Ahead, r.Behind)
}

func withUnpushed(r gitstatus.Repo, label ui.Cell) ui.Cell {
	if r.Unpushed == 0 {
		return label
	}
	return append(ui.Cell{ui.NewSpan(aheadArrow+strconv.Itoa(r.Unpushed)+" ", accent)}, label...)
}

func aheadBehind(ahead, behind int) ui.Cell {
	if ahead == 0 && behind == 0 {
		return ui.Cell{ui.NewSpan(inSync, ui.Success)}
	}
	var parts []ui.Cell
	if ahead > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan(aheadArrow+strconv.Itoa(ahead), accent)})
	}
	if behind > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan(behindArrow+strconv.Itoa(behind), ui.Warning)})
	}
	return ui.JoinCells(parts, " ")
}

func lastCommitCell(r gitstatus.Repo, now time.Time) ui.Cell {
	if r.Unborn {
		return ui.Cell{ui.NewSpan("no commits", ui.Muted)}
	}
	if r.LastCommit.At.IsZero() {
		return nil
	}
	return ui.Cell{ui.NewSpan(ui.Ago(now, r.LastCommit.At), plain)}
}

func notesCell(r gitstatus.Repo) ui.Cell {
	var parts []ui.Cell
	if r.Operation != gitstatus.NoOperation {
		parts = append(parts, ui.Cell{ui.NewSpan(r.Operation.Verb(), ui.Danger)})
	}
	if n := len(r.Stashes); n > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan(ui.Count(n, "stash"), ui.Muted)})
	}
	parts = append(parts, branchNotes(r)...)
	return ui.JoinCells(parts, noteJoiner)
}

// branchNotes names a single noteworthy branch, and only counts several: the details box has more.
func branchNotes(r gitstatus.Repo) []ui.Cell {
	switch len(r.Branches) {
	case 0:
		return nil
	case 1:
		return []ui.Cell{branchCell(r.Branches[0])}
	}
	return []ui.Cell{{ui.NewSpan(ui.Count(len(r.Branches), "branch"), ui.Muted)}}
}

// branchKinds says why the other branches are worth a look: "4 behind, 2 not pushed".
func branchKinds(r gitstatus.Repo) string {
	counts := r.BranchCounts()
	var kinds []string
	for _, count := range []struct {
		n     int
		label string
	}{
		{counts.Behind, "behind"},
		{counts.Ahead, "ahead"},
		{counts.NotPushed, "not pushed"},
		{counts.Gone, "remote gone"},
	} {
		if count.n > 0 {
			kinds = append(kinds, fmt.Sprintf("%d %s", count.n, count.label))
		}
	}
	return strings.Join(kinds, ", ")
}

// branchCell is a branch with its state, such as "main ⇣2" or "spike not pushed".
func branchCell(b gitstatus.Branch) ui.Cell {
	c := append(ui.TicketCell(b.Name, plain), ui.NewSpan(" ", plain))
	switch {
	case b.Gone:
		return append(c, ui.NewSpan("remote gone", ui.Warning))
	case b.NotPushed():
		return append(c, ui.NewSpan("not pushed", accent))
	}
	return append(c, aheadBehind(b.Ahead, b.Behind)...)
}

// PlainRow is a repository's table row as plain text, for output that is not a terminal.
// fetchErr is why fetching it failed, if it did.
func PlainRow(r gitstatus.Repo, now time.Time, fetchErr string) []string {
	return []string{
		r.Name,
		r.Branch,
		changesCell(r).Text(),
		syncCell(r, syncState{fetchErr: fetchErr}).Text(),
		lastCommitCell(r, now).Text(),
		notesCell(r).Text(),
	}
}
