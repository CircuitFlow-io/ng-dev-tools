package tui

import (
	"image/color"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth = 2
	// glyphWidth fits the spinner's two-column moon and a space.
	glyphWidth    = 3
	columnGap     = 2
	minRefWidth   = 8
	maxRefWidth   = 28
	minTitleWidth = 16
	maxStateWidth = 28
)

// group is one section of the list, such as the pull requests waiting for your review.
type group struct {
	title string
	empty string
	prs   []pulls.PR
}

// line is one line of the list: a group header, an empty group's message or a pull request.
type line struct {
	text string
	// pr is the index of the pull request across groups, or -1.
	pr int
}

// list shows the groups with a cursor on the pull requests, scrolling by line so the headers
// scroll with their rows.
type list struct {
	groups    []group
	viewer    string
	cursor    ui.ListCursor
	offset    int
	width     int
	height    int
	now       time.Time
	highlight color.Color
	// runningFrame is the spinner's current frame, shown as the icon of pull requests whose checks
	// are still running.
	runningFrame string
}

func newList(d pulls.Dashboard) list {
	l := list{
		viewer: d.Viewer,
		groups: []group{
			{title: "NEEDS YOUR REVIEW", empty: "Nothing is waiting for your review", prs: d.ToReview},
			{title: "YOURS", empty: "You have no open pull requests", prs: d.Mine},
		},
		height: 1,
		now:    time.Now(),
	}
	l.cursor = ui.NewListCursor(len(l.all()), 0)
	return l
}

// all is every pull request in list order.
func (l list) all() []pulls.PR {
	var all []pulls.PR
	for _, g := range l.groups {
		all = append(all, g.prs...)
	}
	return all
}

func (l list) hasRunningChecks() bool {
	return slices.ContainsFunc(l.all(), pulls.PR.HasRunningChecks)
}

func (l list) current() (pulls.PR, bool) {
	all := l.all()
	if len(all) == 0 {
		return pulls.PR{}, false
	}
	return all[l.cursor.Index], true
}

// keepCurrent moves the cursor to the pull request at url, when it is still listed.
func (l *list) keepCurrent(url string) {
	if i := slices.IndexFunc(l.all(), func(p pulls.PR) bool { return p.URL == url }); i >= 0 {
		l.cursor.MoveTo(i)
	}
	l.follow()
}

func (l *list) resize(width, height int) {
	l.width = width
	l.height = max(height, 1)
	l.follow()
}

func (l *list) handleKey(key string) bool {
	if !l.cursor.HandleKey(key) {
		return false
	}
	l.follow()
	return true
}

// follow scrolls so the cursor's row is visible, with its group header when it is the first row.
func (l *list) follow() {
	lines := l.lines(layout{})
	at := slices.IndexFunc(lines, func(ln line) bool { return ln.pr == l.cursor.Index })
	if at < 0 {
		l.offset = 0
		return
	}
	if at > 0 && lines[at-1].pr < 0 {
		at--
	}
	switch {
	case at < l.offset:
		l.offset = at
	case at >= l.offset+l.height:
		l.offset = at - l.height + 1
	}
	l.offset = max(0, min(l.offset, len(lines)-l.height))
}

// lines renders the whole list; view shows the visible part.
func (l list) lines(lay layout) []line {
	var lines []line
	index := 0
	for _, g := range l.groups {
		lines = append(lines, line{text: ui.Heading.Render(g.title) + ui.Muted.Render("  "+strconv.Itoa(len(g.prs))), pr: -1})
		if len(g.prs) == 0 {
			lines = append(lines, line{text: ui.Muted.Render("  " + g.empty), pr: -1})
		}
		for _, p := range g.prs {
			lines = append(lines, line{text: l.row(p, index, lay), pr: index})
			index++
		}
	}
	return lines
}

func (l list) view() string {
	lines := l.lines(l.layout())
	end := min(len(lines), l.offset+l.height)
	visible := make([]string, 0, end-l.offset)
	for _, ln := range lines[l.offset:end] {
		visible = append(visible, ln.text)
	}
	return strings.Join(visible, "\n")
}

type layout struct {
	ref, title, state, ci, size, updated int
}

// layout fits the columns to their content and gives the title what is left. When that is too
// little, the size and then the CI columns are left out, and at last the reference narrows.
func (l list) layout() layout {
	all := l.all()
	fit := func(field func(pulls.PR) ui.Cell, least, most int) int {
		longest := 0
		for _, p := range all {
			longest = max(longest, field(p).Width())
		}
		return min(most, max(least, longest+columnGap))
	}
	lay := layout{
		ref:     fit(func(p pulls.PR) ui.Cell { return refCell(p, plain) }, minRefWidth, maxRefWidth),
		state:   fit(stateCell, 0, maxStateWidth),
		ci:      fit(ciCell, 0, l.width),
		size:    fit(sizeCell, 0, l.width),
		updated: fit(func(p pulls.PR) ui.Cell { return updatedCell(p, l.now) }, 0, l.width),
	}
	for _, drop := range []*int{&lay.size, &lay.ci} {
		if l.titleRoom(lay) >= minTitleWidth {
			break
		}
		*drop = 0
	}
	if short := minTitleWidth - l.titleRoom(lay); short > 0 {
		lay.ref = max(minRefWidth, lay.ref-short)
	}
	lay.title = max(minTitleWidth, l.titleRoom(lay))
	return lay
}

func (l list) titleRoom(lay layout) int {
	return l.width - cursorWidth - glyphWidth - lay.ref - lay.state - lay.ci - lay.size - lay.updated
}

func (l list) row(p pulls.PR, index int, lay layout) string {
	painter := ui.NewRowPainter(index == l.cursor.Index, l.highlight)
	refStyle, titleStyle := plain, plain
	if isStale(p, l.now) {
		refStyle, titleStyle = ui.Muted, ui.Muted
	}
	if painter.IsHighlighted() {
		refStyle, titleStyle = refStyle.Bold(true), titleStyle.Bold(true)
	}
	row := painter.Cursor() +
		glyphCell(p, l.runningFrame).Render(painter, glyphWidth) +
		refCell(p, refStyle).Truncate(lay.ref-columnGap).Render(painter, lay.ref) +
		titleCell(p, l.viewer, titleStyle).Truncate(lay.title-columnGap).Render(painter, lay.title) +
		stateCell(p).Render(painter, lay.state) +
		optional(ciCell(p), painter, lay.ci) +
		optional(sizeCell(p), painter, lay.size) +
		updatedCell(p, l.now).Render(painter, lay.updated)
	return painter.Fill(row, l.width)
}

// optional renders a column that narrow terminals leave out.
func optional(c ui.Cell, painter ui.RowPainter, width int) string {
	if width == 0 {
		return ""
	}
	return c.Render(painter, width)
}
