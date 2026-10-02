package tui

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	passMark    = "✓"
	failMark    = "✖"
	pendingMark = "◌"
	minusSign   = "−"
	noteJoiner  = " · "
	// staleAfter is how long without an update makes a pull request look abandoned.
	staleAfter = 180 * 24 * time.Hour
)

var (
	accent = lipgloss.NewStyle().Foreground(ui.ColorAccent)
	plain  = lipgloss.NewStyle()
)

type glyph struct {
	symbol string
	style  lipgloss.Style
}

var glyphs = map[pulls.Status]glyph{
	pulls.Conflicts:         {failMark, ui.Danger},
	pulls.ChecksFailing:     {failMark, ui.Danger},
	pulls.NeedsChanges:      {"●", ui.Warning},
	pulls.Draft:             {"◦", ui.Muted},
	pulls.ChecksRunning:     {pendingMark, accent},
	pulls.CheckingConflicts: {pendingMark, ui.Muted},
	pulls.Ready:             {passMark, ui.Success},
	pulls.Waiting:           {"○", ui.Muted},
}

func isStale(p pulls.PR, now time.Time) bool {
	return now.Sub(p.UpdatedAt) > staleAfter
}

// glyphCell is the status icon, spinning in the status's color while checks are still running.
func glyphCell(p pulls.PR, runningFrame string) ui.Cell {
	g := glyphs[p.Status()]
	if runningFrame != "" && p.HasRunningChecks() {
		return ui.Cell{ui.NewSpan(runningFrame, g.style)}
	}
	return ui.Cell{ui.NewSpan(g.symbol, g.style)}
}

// refCell names the pull request by repository and number; the owner is in the details box.
func refCell(p pulls.PR, style lipgloss.Style) ui.Cell {
	_, name, _ := strings.Cut(p.Repo, "/")
	return ui.Cell{ui.NewSpan(name+"#"+strconv.Itoa(p.Number), style)}
}

// titleCell is the title, followed by the author when it is someone else's pull request.
func titleCell(p pulls.PR, viewer string, style lipgloss.Style) ui.Cell {
	c := ui.TicketCell(p.Title, style)
	if p.Author != viewer {
		c = append(c, ui.NewSpan("  @"+p.Author, ui.Muted))
	}
	return c
}

// stateCell says what the pull request is waiting for, in words.
func stateCell(p pulls.PR) ui.Cell {
	switch p.Status() {
	case pulls.Conflicts:
		return ui.Cell{ui.NewSpan("conflicts", ui.Danger)}
	case pulls.ChecksFailing:
		return ui.Cell{ui.NewSpan("checks failing", ui.Danger)}
	case pulls.NeedsChanges:
		return ui.Cell{ui.NewSpan("changes requested", ui.Warning)}
	case pulls.Draft:
		return ui.Cell{ui.NewSpan("draft", ui.Muted)}
	case pulls.ChecksRunning:
		return ui.Cell{ui.NewSpan("checks running", accent)}
	case pulls.CheckingConflicts:
		return ui.Cell{ui.NewSpan("checking conflicts", ui.Muted)}
	case pulls.Ready:
		return ui.Cell{ui.NewSpan("ready to merge", ui.Success)}
	}
	return ui.Cell{ui.NewSpan(waitingFor(p), ui.Muted)}
}

// StateText says what the pull request is waiting for, as the list shows it: "conflicts",
// "checks failing", "ready to merge", "waiting on kim" and so on.
func StateText(p pulls.PR) string {
	return stateCell(p).Text()
}

func waitingFor(p pulls.PR) string {
	if len(p.Requested) > 0 {
		return "waiting on " + strings.Join(p.Requested, ", ")
	}
	return "review required"
}

// ciCell counts the checks: "✓ 5", "✖ 1/5" failed, "◌ 2/5" still running, or "–" for none.
func ciCell(p pulls.PR) ui.Cell {
	counts := p.CheckCounts()
	total := strconv.Itoa(counts.Total())
	switch {
	case counts.Total() == 0:
		return ui.Cell{ui.NewSpan("–", ui.Muted)}
	case counts.Failed > 0:
		return ui.Cell{ui.NewSpan(failMark+" "+strconv.Itoa(counts.Failed)+"/"+total, ui.Danger)}
	case counts.Pending > 0:
		return ui.Cell{ui.NewSpan(pendingMark+" "+strconv.Itoa(counts.Pending)+"/"+total, accent)}
	}
	return ui.Cell{ui.NewSpan(passMark+" "+total, ui.Success)}
}

func sizeCell(p pulls.PR) ui.Cell {
	return ui.Cell{
		ui.NewSpan("+"+strconv.Itoa(p.Additions), ui.Success),
		ui.NewSpan(" "+minusSign+strconv.Itoa(p.Deletions), ui.Danger),
	}
}

func updatedCell(p pulls.PR, now time.Time) ui.Cell {
	return ui.Cell{ui.NewSpan(ui.Ago(now, p.UpdatedAt), plain)}
}

// PlainRow is a pull request's row as plain text, for output that is not a terminal.
func PlainRow(p pulls.PR, viewer string, now time.Time) []string {
	return []string{
		p.Ref(),
		titleCell(p, viewer, plain).Text(),
		stateCell(p).Text(),
		ciCell(p).Text(),
		updatedCell(p, now).Text(),
		p.URL,
	}
}
