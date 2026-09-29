// Package projectlist is the filterable project list shared by the open and run screens.
package projectlist

import (
	"image/color"
	"strings"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth    = 2
	markerWidth    = 2
	activityWidth  = 22
	minNameWidth   = 12
	maxNameWidth   = 32
	minBranchWidth = 8
	maxBranchWidth = 36
	minPathWidth   = 10
	columnGap      = 2
	dirtyMarker    = "●"
)

var branchStyle = lipgloss.NewStyle().Foreground(ui.ColorAccent)

// List is a filterable, scrollable list of projects.
type List struct {
	all         []projects.Project
	shown       []projects.Project
	query       string
	dirty       map[string]bool
	cursor      ui.ListCursor
	width       int
	height      int
	nameWidth   int
	branchWidth int
	home        string
	now         time.Time
	usedVerb    string
	highlight   color.Color
}

// New lists all. usedVerb describes Project.Opened in the activity column, such as "opened".
func New(all []projects.Project, home string, now time.Time, usedVerb string) List {
	l := List{
		all:         all,
		dirty:       map[string]bool{},
		height:      1,
		nameWidth:   columnWidth(all, func(p projects.Project) string { return p.Name }, minNameWidth, maxNameWidth),
		branchWidth: columnWidth(all, func(p projects.Project) string { return p.Branch }, minBranchWidth, maxBranchWidth),
		home:        home,
		now:         now,
		usedVerb:    usedVerb,
	}
	l.SetQuery("")
	return l
}

// columnWidth fits the longest value of field, within limits.
func columnWidth(all []projects.Project, field func(projects.Project) string, least, most int) int {
	longest := 0
	for _, p := range all {
		longest = max(longest, lipgloss.Width(field(p)))
	}
	return min(most, max(least, longest+columnGap))
}

// SetQuery filters the list and puts the cursor on the best match, the most recent one.
func (l *List) SetQuery(query string) {
	l.query = query
	l.shown = projects.Filter(l.all, query)
	l.cursor = ui.NewListCursor(len(l.shown), 0)
	l.cursor.Resize(l.height)
}

// Query is the current filter.
func (l List) Query() string {
	return l.query
}

// Shown is the projects left after filtering.
func (l List) Shown() []projects.Project {
	return l.shown
}

// Len is the number of projects before filtering.
func (l List) Len() int {
	return len(l.all)
}

// Resize sets the space the rows may take.
func (l *List) Resize(width, height int) {
	l.width = width
	l.height = max(height, 1)
	l.cursor.Resize(l.height)
}

// SetHighlight sets the background of the cursor row.
func (l *List) SetHighlight(c color.Color) {
	l.highlight = c
}

// SetDirty marks whether the project at path has uncommitted changes.
func (l *List) SetDirty(path string, dirty bool) {
	l.dirty[path] = dirty
}

// Current is the project under the cursor, if any is shown.
func (l List) Current() (projects.Project, bool) {
	if len(l.shown) == 0 {
		return projects.Project{}, false
	}
	return l.shown[l.cursor.Index], true
}

// HandleKey applies a filter edit or a navigation key and reports whether it was one. Letters go
// to the filter, so j and k do not move the cursor here.
func (l *List) HandleKey(key tea.KeyPressMsg) bool {
	if key.String() == "backspace" {
		l.SetQuery(dropLastRune(l.query))
		return true
	}
	if IsTyping(key) {
		l.SetQuery(l.query + key.Text)
		return true
	}
	return l.cursor.HandleKey(key.String())
}

// IsTyping reports whether key is a character for a filter rather than a shortcut.
func IsTyping(key tea.KeyPressMsg) bool {
	return key.Text != "" && key.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModSuper) == 0
}

func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// FilterLine is the prompt showing the filter being typed.
func (l List) FilterLine() string {
	return FilterLine(l.query, "type to filter")
}

// FilterLine renders a filter prompt, showing placeholder while query is empty.
func FilterLine(query, placeholder string) string {
	prompt := ui.Selected.Render("> ")
	if query == "" {
		return prompt + ui.Muted.Render(placeholder)
	}
	return prompt + query + ui.Selected.Render("▏")
}

// columns fits the branch and path columns into the width left over: the branch shrinks first,
// then the path is left out.
func (l List) columns() (branch, path int) {
	rest := l.width - cursorWidth - l.nameWidth - markerWidth - activityWidth
	branch = min(l.branchWidth, max(minBranchWidth, rest))
	path = rest - branch
	if path < minPathWidth {
		path = 0
	}
	return branch, path
}

// Header is the column titles line.
func (l List) Header() string {
	branch, path := l.columns()
	cells := strings.Repeat(" ", cursorWidth) + ui.PadRight("PROJECT", l.nameWidth+markerWidth) +
		ui.PadRight("BRANCH", branch) + ui.PadRight("LAST ACTIVITY", activityWidth)
	if path > 0 {
		cells += "PATH"
	}
	return ui.Muted.Render(cells)
}

// View renders the visible rows.
func (l List) View() string {
	if len(l.shown) == 0 {
		return ui.Muted.Render("  No projects match")
	}
	start, end := l.cursor.Visible()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, l.renderRow(i))
	}
	return strings.Join(lines, "\n")
}

func (l List) renderRow(index int) string {
	p := l.shown[index]
	painter := ui.NewRowPainter(index == l.cursor.Index, l.highlight)

	nameStyle := lipgloss.NewStyle().Width(l.nameWidth)
	if painter.IsHighlighted() {
		nameStyle = nameStyle.Bold(true)
	}
	marker := painter.Paint(lipgloss.NewStyle().Width(markerWidth), "")
	if l.dirty[p.Path] {
		marker = painter.Paint(ui.Warning.Width(markerWidth), dirtyMarker)
	}
	branchWidth, pathWidth := l.columns()

	row := painter.Cursor() +
		painter.Paint(nameStyle, ui.Truncate(p.Name, l.nameWidth-columnGap)) +
		marker +
		painter.Paint(branchStyle.Width(branchWidth), ui.Truncate(p.Branch, branchWidth-columnGap)) +
		painter.Paint(lipgloss.NewStyle().Width(activityWidth), l.activity(p))
	if pathWidth > 0 {
		row += painter.Paint(ui.Muted.Width(pathWidth), ui.TruncatePath(ui.TildePath(p.Path, l.home), pathWidth-1))
	}
	return painter.Fill(row, l.width)
}

func (l List) activity(p projects.Project) string {
	if p.LastActivity().IsZero() {
		return ""
	}
	verb := "changed"
	if p.Opened.After(p.Changed) {
		verb = l.usedVerb
	}
	return verb + " " + ui.Ago(l.now, p.LastActivity())
}
