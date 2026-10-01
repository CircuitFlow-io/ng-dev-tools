package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// detailLines is the fixed height of the details box's content, so the list does not jump.
	detailLines      = 8
	detailFrameWidth = 4
	// twoColumnWidth is the terminal width from which checks and reviews sit side by side.
	twoColumnWidth  = 100
	columnSeparator = " │ "
)

var (
	detailBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.ColorAccent).Padding(0, 1)
	sectionLabel = ui.Heading
)

// details describes the pull request under the cursor. clone is its local project, if any.
func details(p pulls.PR, ok bool, clone string, home string, now time.Time, width int) string {
	inner := width - detailFrameWidth
	if !ok {
		return detailBox.Width(width).Render(padLines([]string{ui.Muted.Render("No pull request selected")}))
	}
	lines := []string{
		ui.FitLine(ui.Bold.Render(p.Ref())+"  "+ui.RenderTickets(p.Title, plain), inner),
		ui.FitLine(origin(p, now), inner),
		ui.FitLine(sizeLine(p, clone, home), inner),
	}
	lines = append(lines, body(p, inner, detailLines-len(lines))...)
	return detailBox.Width(width).Render(padLines(lines))
}

func origin(p pulls.PR, now time.Time) string {
	return ui.Muted.Render("@"+p.Author+" wants to merge ") + ui.RenderTickets(p.HeadRef, ui.Muted) +
		ui.Muted.Render(fmt.Sprintf(" into %s · opened %s · updated %s", p.BaseRef, ui.Ago(now, p.CreatedAt), ui.Ago(now, p.UpdatedAt)))
}

func sizeLine(p pulls.PR, clone, home string) string {
	facts := []string{ui.Count(p.ChangedFiles, "file"), ui.Count(p.Comments, "comment")}
	if clone != "" {
		facts = append(facts, "cloned in "+ui.TildePath(clone, home))
	} else {
		facts = append(facts, "not cloned locally")
	}
	return sizeCell(p).String() + ui.Muted.Render(" in "+strings.Join(facts, noteJoiner))
}

// body puts the checks beside the review and merge state, or above them when narrow.
func body(p pulls.PR, width, height int) []string {
	checks := []ui.Section{checksSection(p)}
	review := []ui.Section{reviewSection(p), mergeSection(p)}
	if width+detailFrameWidth < twoColumnWidth {
		return ui.FitSections(append(checks, review...), width, height)
	}
	separatorWidth := lipgloss.Width(columnSeparator)
	leftWidth := (width - separatorWidth) / 2
	rightWidth := width - separatorWidth - leftWidth
	left := ui.FitSections(checks, leftWidth, height)
	right := ui.FitSections(review, rightWidth, height)
	return ui.SideBySide(left, right, leftWidth, columnSeparator)
}

func checksSection(p pulls.PR) ui.Section {
	counts := p.CheckCounts()
	if counts.Total() == 0 {
		return ui.Section{Title: sectionLabel.Render("CHECKS") + "  " + ui.Muted.Render("none")}
	}
	var summary []string
	for _, count := range []struct {
		n     int
		label string
	}{{counts.Failed, "failed"}, {counts.Pending, "running"}, {counts.Passed, "passed"}, {counts.Skipped, "skipped"}} {
		if count.n > 0 {
			summary = append(summary, fmt.Sprintf("%d %s", count.n, count.label))
		}
	}
	s := ui.Section{Title: sectionLabel.Render("CHECKS") + "  " + strings.Join(summary, noteJoiner)}
	for _, check := range p.Checks {
		s.Items = append(s.Items, checkLine(check))
	}
	return s
}

var checkMarks = map[pulls.CheckState]glyph{
	pulls.Failed:  {failMark, ui.Danger},
	pulls.Pending: {pendingMark, accent},
	pulls.Passed:  {passMark, ui.Success},
	pulls.Skipped: {"-", ui.Muted},
}

func checkLine(check pulls.Check) string {
	mark := checkMarks[check.State]
	line := mark.style.Render(mark.symbol) + " " + check.Name
	if check.Workflow != "" {
		line += ui.Muted.Render("  " + check.Workflow)
	}
	return line
}

func reviewSection(p pulls.PR) ui.Section {
	s := ui.Section{Title: sectionLabel.Render("REVIEW") + "  " + reviewDecision(p)}
	for _, r := range p.Reviews {
		s.Items = append(s.Items, reviewLine(r))
	}
	for _, reviewer := range p.Requested {
		s.Items = append(s.Items, ui.Muted.Render("○ waiting on "+reviewer))
	}
	return s
}

func reviewDecision(p pulls.PR) string {
	switch p.ReviewDecision {
	case pulls.Approved:
		return ui.Success.Render("approved")
	case pulls.ChangesRequested:
		return ui.Warning.Render("changes requested")
	case pulls.ReviewRequired:
		return ui.Muted.Render("review required")
	}
	return ui.Muted.Render("not required")
}

func reviewLine(r pulls.Review) string {
	switch r.State {
	case pulls.Approved:
		return ui.Success.Render(passMark) + " approved by " + r.Author
	case pulls.ChangesRequested:
		return ui.Warning.Render("●") + " changes requested by " + r.Author
	}
	return ui.Muted.Render("○") + " " + strings.ToLower(r.State) + " by " + r.Author
}

func mergeSection(p pulls.PR) ui.Section {
	label := sectionLabel.Render("MERGE") + "  "
	switch {
	case p.Conflicting():
		return ui.Section{Title: label + ui.Danger.Render("conflicts with "+p.BaseRef)}
	case p.Draft:
		return ui.Section{Title: label + ui.Muted.Render("draft, not ready yet")}
	case p.NoConflicts():
		return ui.Section{Title: label + ui.Success.Render("no conflicts with "+p.BaseRef)}
	}
	return ui.Section{Title: label + ui.Muted.Render("GitHub is still checking for conflicts")}
}

func padLines(lines []string) string {
	lines = lines[:min(len(lines), detailLines)]
	for len(lines) < detailLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
