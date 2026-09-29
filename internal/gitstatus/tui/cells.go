package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/gitstatus"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
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

// span is a run of text in one style; a cell is the spans of one table cell.
type span struct {
	text  string
	style lipgloss.Style
}

type cell []span

func (c cell) width() int {
	total := 0
	for _, s := range c {
		total += lipgloss.Width(s.text)
	}
	return total
}

// render paints the cell within width, cutting the text short when it does not fit.
func (c cell) render(painter ui.RowPainter, width int) string {
	var b strings.Builder
	room := width
	for _, s := range c {
		if room <= 0 {
			break
		}
		text := ui.Truncate(s.text, room)
		room -= lipgloss.Width(text)
		b.WriteString(painter.Paint(s.style, text))
	}
	if room > 0 {
		b.WriteString(painter.Paint(plain, strings.Repeat(" ", room)))
	}
	return b.String()
}

func joinCells(cells []cell, joiner string) cell {
	var joined cell
	for i, c := range cells {
		if i > 0 {
			joined = append(joined, span{joiner, ui.Muted})
		}
		joined = append(joined, c...)
	}
	return joined
}

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

func glyphCell(r gitstatus.Repo) cell {
	g := glyphs[r.Attention()]
	return cell{{g.symbol, g.style}}
}

func changesCell(r gitstatus.Repo) cell {
	if r.Err != nil {
		return cell{{"unreadable", ui.Danger}}
	}
	c := r.Changes()
	if !c.Any() {
		return cell{{"clean", ui.Success}}
	}
	var parts []cell
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
			parts = append(parts, cell{{count.prefix + strconv.Itoa(count.n), count.style}})
		}
	}
	return joinCells(parts, " ")
}

// syncState is what the table knows about a fetch of the repository.
type syncState struct {
	fetching bool
	spinner  string
	fetchErr string
}

func syncCell(r gitstatus.Repo, s syncState) cell {
	switch {
	case s.fetching:
		return cell{{s.spinner, ui.Title}, {" fetching", ui.Muted}}
	case s.fetchErr != "":
		return cell{{"fetch failed", ui.Danger}}
	case r.Err != nil:
		return nil
	case !r.HasRemote:
		return cell{{"no remote", ui.Muted}}
	case r.Unborn:
		return cell{{"no commits", ui.Muted}}
	case r.Detached:
		return cell{{"detached", ui.Muted}}
	case r.UpstreamGone:
		return withUnpushed(r, cell{{"remote gone", ui.Warning}})
	case r.NotPushed():
		return withUnpushed(r, cell{{"not pushed", accent}})
	}
	return aheadBehind(r.Ahead, r.Behind)
}

func withUnpushed(r gitstatus.Repo, label cell) cell {
	if r.Unpushed == 0 {
		return label
	}
	return append(cell{{aheadArrow + strconv.Itoa(r.Unpushed) + " ", accent}}, label...)
}

func aheadBehind(ahead, behind int) cell {
	if ahead == 0 && behind == 0 {
		return cell{{inSync, ui.Success}}
	}
	var parts []cell
	if ahead > 0 {
		parts = append(parts, cell{{aheadArrow + strconv.Itoa(ahead), accent}})
	}
	if behind > 0 {
		parts = append(parts, cell{{behindArrow + strconv.Itoa(behind), ui.Warning}})
	}
	return joinCells(parts, " ")
}

func lastCommitCell(r gitstatus.Repo, now time.Time) cell {
	if r.Unborn {
		return cell{{"no commits", ui.Muted}}
	}
	if r.LastCommit.At.IsZero() {
		return nil
	}
	return cell{{ui.Ago(now, r.LastCommit.At), plain}}
}

func notesCell(r gitstatus.Repo) cell {
	var parts []cell
	if r.Operation != gitstatus.NoOperation {
		parts = append(parts, cell{{r.Operation.Verb(), ui.Danger}})
	}
	if n := len(r.Stashes); n > 0 {
		parts = append(parts, cell{{ui.Count(n, "stash"), ui.Muted}})
	}
	parts = append(parts, branchNotes(r)...)
	return joinCells(parts, noteJoiner)
}

// branchNotes names a single noteworthy branch, and only counts several: the details box has more.
func branchNotes(r gitstatus.Repo) []cell {
	switch len(r.Branches) {
	case 0:
		return nil
	case 1:
		return []cell{branchCell(r.Branches[0])}
	}
	return []cell{{{ui.Count(len(r.Branches), "branch"), ui.Muted}}}
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
func branchCell(b gitstatus.Branch) cell {
	c := cell{{b.Name + " ", plain}}
	switch {
	case b.Gone:
		return append(c, span{"remote gone", ui.Warning})
	case b.NotPushed():
		return append(c, span{"not pushed", accent})
	}
	return append(c, aheadBehind(b.Ahead, b.Behind)...)
}

// text is the cell without styling.
func (c cell) text() string {
	var b strings.Builder
	for _, s := range c {
		b.WriteString(s.text)
	}
	return b.String()
}

// PlainRow is a repository's table row as plain text, for output that is not a terminal.
// fetchErr is why fetching it failed, if it did.
func PlainRow(r gitstatus.Repo, now time.Time, fetchErr string) []string {
	return []string{
		r.Name,
		r.Branch,
		changesCell(r).text(),
		syncCell(r, syncState{fetchErr: fetchErr}).text(),
		lastCommitCell(r, now).text(),
		notesCell(r).text(),
	}
}
