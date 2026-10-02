package tui

import (
	"image/color"
	"slices"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/projectlist"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	minProjectWidth = 16
	maxProjectWidth = 28
)

// projectColumn is the filterable list of projects on the left of the screen.
type projectColumn struct {
	all    []projects.Project
	shown  []projects.Project
	query  string
	cursor ui.ListCursor
	height int
}

// newProjectColumn lists all with the cursor on the project at selected, or the first one.
func newProjectColumn(all []projects.Project, selected string) projectColumn {
	c := projectColumn{all: all, shown: all, height: 1}
	c.cursor = ui.NewListCursor(len(all), max(0, c.indexOf(selected)))
	return c
}

func (c projectColumn) indexOf(path string) int {
	return slices.IndexFunc(c.shown, func(p projects.Project) bool { return p.Path == path })
}

func (c projectColumn) current() (projects.Project, bool) {
	if len(c.shown) == 0 {
		return projects.Project{}, false
	}
	return c.shown[c.cursor.Index], true
}

// setQuery filters the list and puts the cursor on the best match, the most recent one.
func (c *projectColumn) setQuery(query string) {
	c.query = query
	c.shown = projects.Filter(c.all, query)
	c.cursor = ui.NewListCursor(len(c.shown), 0)
	c.cursor.Resize(c.height)
}

// handleKey applies a filter edit or a move, and reports whether it was one.
func (c *projectColumn) handleKey(key tea.KeyPressMsg) bool {
	switch {
	case key.String() == "backspace":
		if c.query == "" {
			return false
		}
		c.setQuery(dropLastRune(c.query))
		return true
	case projectlist.IsTyping(key):
		c.setQuery(c.query + key.Text)
		return true
	}
	return c.cursor.HandleKey(key.String())
}

func (c *projectColumn) resize(height int) {
	c.height = max(height, 1)
	c.cursor.Resize(c.height)
}

func (c projectColumn) width() int {
	longest := 0
	for _, p := range c.all {
		longest = max(longest, lipgloss.Width(p.Name))
	}
	return min(maxProjectWidth, max(minProjectWidth, cursorWidth+longest+columnGap))
}

func (c projectColumn) rows(focused bool, highlight color.Color) []string {
	if len(c.shown) == 0 {
		return []string{ui.Muted.Render("  No projects match")}
	}
	width := c.width()
	nameWidth := width - cursorWidth - columnGap
	start, end := c.cursor.Visible()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		name := ui.Truncate(c.shown[i].Name, nameWidth)
		lines = append(lines, columnRow(i == c.cursor.Index, focused, highlight, name, width-columnGap))
	}
	return lines
}

// columnRow renders a one-field row of a column: the cursor row is highlighted across the
// column while it has focus, and only marked with ▸ while another column has it.
func columnRow(atCursor, focused bool, highlight color.Color, text string, width int) string {
	switch {
	case atCursor && focused:
		painter := ui.NewRowPainter(true, highlight)
		return painter.Fill(painter.Cursor()+painter.Paint(ui.Bold, text), width)
	case atCursor:
		return ui.Selected.Render("▸ " + text)
	default:
		return "  " + text
	}
}
