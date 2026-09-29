package tui

import (
	"fmt"
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/standup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth  = 2
	groupIndent  = 2
	commitIndent = 4
	rightGap     = 2
	noteJoiner   = " · "
	clockFormat  = "15:04"
	weekFormat   = "Mon 15:04"
	dateFormat   = "Mon 2 Jan"
	recentDays   = 6
)

var (
	accent = lipgloss.NewStyle().Foreground(ui.ColorAccent)
	plain  = lipgloss.NewStyle()
)

type rowKind int

const (
	blankRow rowKind = iota
	sectionRow
	projectRow
	groupRow
	commitRow
	reviewedRow
	inProgressRow
)

// row is one line of the report: its text on the left, facts about it on the right, and what o
// opens.
type row struct {
	kind   rowKind
	indent int
	left   ui.Cell
	right  ui.Cell
	url    string
	// what names the thing url points to, for the flash line.
	what string
}

func (r row) selectable() bool {
	return r.kind != blankRow && r.kind != sectionRow
}

// buildRows lays out the report: each project with its pull requests and branches and their
// commits, then the pull requests you reviewed, then the work in progress.
func buildRows(r standup.Report, now time.Time) []row {
	var rows []row
	for _, p := range r.Projects {
		rows = appendBlock(rows, projectRows(p, r.Since, now))
	}
	if len(r.Reviewed) > 0 {
		rows = appendBlock(rows, reviewedRows(r.Reviewed))
	}
	if len(r.InProgress) > 0 {
		rows = appendBlock(rows, inProgressRows(r.InProgress))
	}
	return rows
}

// appendBlock adds block after rows, with a blank line between them.
func appendBlock(rows, block []row) []row {
	if len(rows) > 0 {
		rows = append(rows, row{kind: blankRow})
	}
	return append(rows, block...)
}

func projectRows(p standup.Project, since, now time.Time) []row {
	rows := []row{{
		kind:  projectRow,
		left:  ui.Cell{ui.NewSpan(p.Name, ui.Bold)},
		right: projectFacts(p),
		url:   p.URL,
		what:  p.Name,
	}}
	for _, g := range p.Groups {
		rows = append(rows, groupRowFor(g, p, since))
		for _, c := range g.Commits {
			rows = append(rows, row{
				kind:   commitRow,
				indent: commitIndent,
				left:   ui.Cell{ui.NewSpan(c.Short()+" ", ui.Muted), ui.NewSpan(c.Subject, plain)},
				right:  ui.Cell{ui.NewSpan(when(c.At, now), ui.Muted)},
				url:    c.URL,
				what:   c.Short(),
			})
		}
	}
	return rows
}

func projectFacts(p standup.Project) ui.Cell {
	commits, prs := 0, 0
	for _, g := range p.Groups {
		commits += len(g.Commits)
		if g.PR != nil {
			prs++
		}
	}
	var facts []string
	if commits > 0 {
		facts = append(facts, ui.Count(commits, "commit"))
	}
	if prs > 0 {
		facts = append(facts, ui.Count(prs, "PR"))
	}
	if p.Dir == "" {
		facts = append(facts, "not cloned")
	}
	return ui.Cell{ui.NewSpan(strings.Join(facts, noteJoiner), ui.Muted)}
}

func groupRowFor(g standup.Group, p standup.Project, since time.Time) row {
	if g.PR != nil {
		return row{
			kind:   groupRow,
			indent: groupIndent,
			left:   ui.Cell{ui.NewSpan(fmt.Sprintf("#%d ", g.PR.Number), accent), ui.NewSpan(g.PR.Title, plain)},
			right:  prEvents(*g.PR, since),
			url:    g.PR.URL,
			what:   fmt.Sprintf("#%d", g.PR.Number),
		}
	}
	if g.Branch == "" {
		return row{
			kind:   groupRow,
			indent: groupIndent,
			left:   ui.Cell{ui.NewSpan(g.DefaultBranch, plain)},
			right:  ui.Cell{ui.NewSpan("direct commits", ui.Muted)},
			url:    treeURL(p, g.DefaultBranch),
			what:   g.DefaultBranch,
		}
	}
	r := row{kind: groupRow, indent: groupIndent, left: ui.Cell{ui.NewSpan(g.Branch, plain)}, what: g.Branch}
	if pushed(g) {
		r.right = ui.Cell{ui.NewSpan("no PR", ui.Muted)}
		r.url = treeURL(p, g.Branch)
		return r
	}
	r.right = ui.Cell{ui.NewSpan("not pushed", ui.Warning)}
	return r
}

// pushed reports whether the branch's commits are on GitHub.
func pushed(g standup.Group) bool {
	for _, c := range g.Commits {
		if c.URL == "" {
			return false
		}
	}
	return true
}

func treeURL(p standup.Project, branch string) string {
	if p.URL == "" || branch == "" {
		return ""
	}
	return p.URL + "/tree/" + branch
}

// prEvents is what happened to the pull request since the start, or its state when nothing did.
func prEvents(p standup.PR, since time.Time) ui.Cell {
	var events []ui.Cell
	if p.OpenedSince(since) {
		events = append(events, ui.Cell{ui.NewSpan("opened", accent)})
	}
	switch {
	case p.MergedSince(since):
		events = append(events, ui.Cell{ui.NewSpan("merged", ui.Success)})
	case p.ClosedSince(since):
		events = append(events, ui.Cell{ui.NewSpan("closed", ui.Danger)})
	case len(events) == 0:
		events = append(events, ui.Cell{ui.NewSpan(strings.ToLower(p.State), ui.Muted)})
	}
	if p.Draft && p.State == standup.Open {
		events = append(events, ui.Cell{ui.NewSpan("draft", ui.Muted)})
	}
	return ui.JoinCells(events, noteJoiner)
}

func reviewedRows(reviewed []standup.Reviewed) []row {
	rows := []row{{kind: sectionRow, left: ui.Cell{ui.NewSpan("Reviewed", ui.Heading)}}}
	for _, r := range reviewed {
		rows = append(rows, row{
			kind:   reviewedRow,
			indent: groupIndent,
			left: ui.Cell{
				ui.NewSpan(r.Ref()+" ", accent), ui.NewSpan(r.Title, plain), ui.NewSpan(" by "+r.Author, ui.Muted),
			},
			right: verdictCell(r.Verdict),
			url:   r.URL,
			what:  r.Ref(),
		})
	}
	return rows
}

func verdictCell(verdict string) ui.Cell {
	switch verdict {
	case standup.Approved:
		return ui.Cell{ui.NewSpan("approved", ui.Success)}
	case standup.ChangesRequested:
		return ui.Cell{ui.NewSpan("changes requested", ui.Warning)}
	}
	return ui.Cell{ui.NewSpan("commented", ui.Muted)}
}

func inProgressRows(repos []gitstatus.Repo) []row {
	rows := []row{{kind: sectionRow, left: ui.Cell{ui.NewSpan("In progress", ui.Heading)}}}
	for _, r := range repos {
		rows = append(rows, row{
			kind:   inProgressRow,
			indent: groupIndent,
			left:   ui.Cell{ui.NewSpan(r.Name, ui.Bold), ui.NewSpan("  "+r.Branch, plain)},
			right:  progressCell(r),
		})
	}
	return rows
}

// progressCell counts what is not committed or not pushed yet.
func progressCell(r gitstatus.Repo) ui.Cell {
	c := r.Changes()
	var parts []ui.Cell
	for _, part := range []struct {
		n     int
		label string
		style lipgloss.Style
	}{
		{c.Conflicted, "conflicted", ui.Danger},
		{c.Staged, "staged", ui.Success},
		{c.Modified, "modified", ui.Warning},
		{c.Untracked, "untracked", ui.Muted},
		{r.Unpushed, "not pushed", accent},
	} {
		if part.n > 0 {
			parts = append(parts, ui.Cell{ui.NewSpan(fmt.Sprintf("%d %s", part.n, part.label), part.style)})
		}
	}
	if r.NotPushed() && r.Unpushed == 0 {
		parts = append(parts, ui.Cell{ui.NewSpan("branch not pushed", accent)})
	}
	return ui.JoinCells(parts, noteJoiner)
}

// renderRow lays out a row at width: the cursor, the indent, the left text cut to fit, and the
// right facts after it.
func renderRow(r row, highlighted bool, background color.Color, width int) string {
	if r.kind == blankRow {
		return ""
	}
	painter := ui.NewRowPainter(highlighted, background)
	rightWidth := min(r.right.Width(), width/2)
	leftWidth := width - cursorWidth - r.indent
	if rightWidth > 0 {
		leftWidth -= rightWidth + rightGap
	}
	line := painter.Cursor() + painter.Paint(plain, strings.Repeat(" ", r.indent)) + r.left.Render(painter, max(leftWidth, 0))
	if rightWidth > 0 {
		line += painter.Paint(plain, strings.Repeat(" ", rightGap)) + r.right.Render(painter, rightWidth)
	}
	return painter.Fill(line, width)
}

// when is a commit's time: the clock today, the weekday in the last week, the date before.
func when(t, now time.Time) string {
	t = t.In(now.Location())
	today := standup.StartOfDay(now)
	switch {
	case !t.Before(today):
		return t.Format(clockFormat)
	case !t.Before(today.AddDate(0, 0, -recentDays)):
		return t.Format(weekFormat)
	}
	return t.Format(dateFormat)
}

// PlainLines is the report as plain text, for output that is not a terminal.
func PlainLines(r standup.Report, now time.Time) []string {
	lines := []string{plainTitle(r, now), ""}
	if r.Empty() {
		return append(lines, emptyMessage(r, now))
	}
	for _, row := range buildRows(r, now) {
		line := strings.Repeat(" ", row.indent) + row.left.Text()
		if right := row.right.Text(); right != "" {
			line += strings.Repeat(" ", rightGap) + "(" + right + ")"
		}
		lines = append(lines, line)
	}
	return lines
}

func plainTitle(r standup.Report, now time.Time) string {
	title := "Standup since " + sinceLabel(r, now) + noteJoiner + summary(r)
	if r.GitHubErr != nil {
		title += noteJoiner + "GitHub not read: " + r.GitHubErr.Error()
	}
	return title
}

// sinceLabel names the start of the report: "today", "Mon 28 Sep", and why that day when it was
// worked out.
func sinceLabel(r standup.Report, now time.Time) string {
	label := r.Since.Format(dateFormat)
	if r.Since.Equal(standup.StartOfDay(now)) {
		label = "today"
	}
	if r.LastWorkedDay {
		label += ", the last day you worked"
	}
	return label
}

// summary counts the report: commits and repos, pull requests opened and merged, and reviews.
func summary(r standup.Report) string {
	repos := 0
	for _, p := range r.Projects {
		if slices.ContainsFunc(p.Groups, func(g standup.Group) bool { return len(g.Commits) > 0 }) {
			repos++
		}
	}
	facts := []string{"no commits"}
	if n := r.Commits(); n > 0 {
		facts[0] = fmt.Sprintf("%s in %s", ui.Count(n, "commit"), ui.Count(repos, "repo"))
	}
	opened, merged := r.PRs()
	if opened > 0 {
		facts = append(facts, ui.Count(opened, "PR")+" opened")
	}
	if merged > 0 && opened > 0 {
		facts = append(facts, fmt.Sprintf("%d merged", merged))
	} else if merged > 0 {
		facts = append(facts, ui.Count(merged, "PR")+" merged")
	}
	if n := len(r.Reviewed); n > 0 {
		facts = append(facts, fmt.Sprintf("%d reviewed", n))
	}
	return strings.Join(facts, noteJoiner)
}

func emptyMessage(r standup.Report, now time.Time) string {
	return "Nothing since " + sinceLabel(r, now) + ": no commits, pull requests or reviews"
}
