package tui

import (
	"image/color"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth     = 2
	columnGap       = 2
	minPromptWidth  = 24
	minProjectWidth = 10
	maxProjectWidth = 22
	minBranchWidth  = 8
	maxBranchWidth  = 24
	maxModelWidth   = 14
)

var (
	plain       = lipgloss.NewStyle()
	accent      = lipgloss.NewStyle().Foreground(ui.ColorAccent)
	missingDir  = ui.Muted.Strikethrough(true)
	promptCount = lipgloss.NewStyle().Align(lipgloss.Right)
)

// table is the sessions matching the search, with a cursor.
type table struct {
	results   []claudesessions.Result
	missing   map[string]bool
	live      map[string]claudesessions.Live
	cursor    ui.ListCursor
	width     int
	height    int
	now       time.Time
	home      string
	root      string
	highlight color.Color
}

func newTable(home, root string) table {
	return table{height: 1, now: time.Now(), home: home, root: root, missing: map[string]bool{}}
}

// setResults replaces the rows and puts the cursor on the first, the most recent.
func (t *table) setResults(results []claudesessions.Result) {
	t.results = results
	t.cursor = ui.NewListCursor(len(results), 0)
	t.cursor.Resize(t.height)
}

func (t table) current() (claudesessions.Result, bool) {
	if len(t.results) == 0 {
		return claudesessions.Result{}, false
	}
	return t.results[t.cursor.Index], true
}

func (t *table) resize(width, height int) {
	t.width = width
	t.height = max(height, 1)
	t.cursor.Resize(t.height)
}

// projectLabel names a session's folder by its path inside the projects folder, or from home.
func projectLabel(dir, home, root string) string {
	if rel, err := filepath.Rel(root, dir); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return ui.TildePath(dir, home)
}

type layout struct {
	active, status, prompt, project, branch, prompts, model int
}

// layout fits the columns to their content and gives the first prompt what is left. When that is
// too little the model goes first, then the branch, then the project narrows.
func (t table) layout() layout {
	l := layout{
		active:  t.fit("ACTIVE", func(r claudesessions.Result) string { return ui.Ago(t.now, r.Session.LastActive) }, 0, t.width),
		status:  t.fit(statusHead, func(r claudesessions.Result) string { return statusCell(t.live[r.Session.ID]) }, 0, t.width),
		project: t.fit("PROJECT", func(r claudesessions.Result) string { return t.project(r.Session) }, minProjectWidth, maxProjectWidth),
		branch:  t.fit("BRANCH", func(r claudesessions.Result) string { return r.Session.Branch }, minBranchWidth, maxBranchWidth),
		prompts: t.fit("PROMPTS", func(r claudesessions.Result) string { return strconv.Itoa(r.Session.Prompts) }, 0, t.width),
		model:   t.fit("MODEL", func(r claudesessions.Result) string { return modelName(r.Session) }, 0, maxModelWidth),
	}
	l.prompt = t.width - cursorWidth - l.active - l.status - l.project - l.branch - l.prompts - l.model
	for _, column := range []*int{&l.model, &l.branch} {
		if l.prompt < minPromptWidth {
			l.prompt += *column
			*column = 0
		}
	}
	if l.prompt < minPromptWidth {
		shrink := min(minPromptWidth-l.prompt, l.project-minProjectWidth)
		l.project -= shrink
		l.prompt += shrink
	}
	return l
}

func (t table) fit(title string, field func(claudesessions.Result) string, least, most int) int {
	longest := lipgloss.Width(title)
	for _, r := range t.results {
		longest = max(longest, lipgloss.Width(field(r)))
	}
	return min(most, max(least, longest+columnGap))
}

func (t table) project(s claudesessions.Session) string {
	return projectLabel(s.Dir, t.home, t.root)
}

func modelName(s claudesessions.Session) string {
	return claudesessions.ModelName(s.MainModel())
}

func (t table) header() string {
	l := t.layout()
	cells := strings.Repeat(" ", cursorWidth) + ui.PadRight("ACTIVE", l.active) + ui.PadRight(statusHead, l.status) +
		ui.PadRight("FIRST PROMPT", l.prompt) +
		ui.PadRight("PROJECT", l.project)
	if l.branch > 0 {
		cells += ui.PadRight("BRANCH", l.branch)
	}
	cells += ui.PadLeft("PROMPTS", l.prompts-columnGap) + strings.Repeat(" ", columnGap)
	if l.model > 0 {
		cells += "MODEL"
	}
	return ui.Muted.Render(cells)
}

// view renders the visible rows, padded to the table's height when it scrolls, so what follows
// stays in place.
func (t table) view() string {
	if len(t.results) == 0 {
		return ui.Muted.Render("  No sessions match")
	}
	l := t.layout()
	start, end := t.cursor.Visible()
	lines := make([]string, 0, t.height)
	for i := start; i < end; i++ {
		lines = append(lines, t.row(i, l))
	}
	for len(lines) < min(t.height, len(t.results)) {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (t table) row(index int, l layout) string {
	s := t.results[index].Session
	live := t.live[s.ID]
	painter := ui.NewRowPainter(index == t.cursor.Index, t.highlight)
	promptStyle := plain
	if painter.IsHighlighted() {
		promptStyle = ui.Bold
	}
	projectStyle := plain
	if t.missing[s.Dir] {
		projectStyle = missingDir
	}
	row := painter.Cursor() +
		painter.Paint(ui.Muted.Width(l.active), ui.Ago(t.now, s.LastActive)) +
		painter.Paint(statusStyle(live).Width(l.status), statusCell(live)) +
		painter.Paint(promptStyle.Width(l.prompt), ui.Truncate(s.FirstPrompt, l.prompt-columnGap)) +
		painter.Paint(projectStyle.Width(l.project), ui.TruncatePath(t.project(s), l.project-columnGap))
	if l.branch > 0 {
		row += painter.Paint(accent.Width(l.branch), ui.Truncate(s.Branch, l.branch-columnGap))
	}
	row += painter.Paint(promptCount.Width(l.prompts-columnGap), strconv.Itoa(s.Prompts)) + painter.Paint(plain, strings.Repeat(" ", columnGap))
	if l.model > 0 {
		row += painter.Paint(ui.Muted.Width(l.model), ui.Truncate(modelName(s), l.model))
	}
	return painter.Fill(row, t.width)
}
