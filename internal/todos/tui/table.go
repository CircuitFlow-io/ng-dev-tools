package tui

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth     = 2
	columnGap       = 2
	markerWidth     = len("FIXME") + columnGap
	minProjectWidth = 10
	maxProjectWidth = 22
	minFileWidth    = 16
	maxFileWidth    = 36
	maxAuthorWidth  = 20
	minNoteWidth    = 30
)

// table is the list of marker comments with a cursor, all of them or only yours.
type table struct {
	all       []todos.Item
	items     []todos.Item
	mineOnly  bool
	cursor    ui.ListCursor
	width     int
	height    int
	now       time.Time
	highlight color.Color
}

func newTable() table {
	return table{height: 1, now: time.Now()}
}

// setItems replaces the rows, keeping the cursor on the same item when it is still listed.
func (t *table) setItems(items []todos.Item) {
	t.all = items
	t.now = time.Now()
	t.filter()
}

func (t *table) toggleMine() {
	t.mineOnly = !t.mineOnly
	t.filter()
}

func (t *table) filter() {
	current, hadCurrent := t.current()
	t.items = t.all
	if t.mineOnly {
		t.items = slices.DeleteFunc(slices.Clone(t.all), func(i todos.Item) bool { return !i.Mine })
	}
	index := 0
	if hadCurrent {
		index = max(0, slices.IndexFunc(t.items, func(i todos.Item) bool { return i.ID() == current.ID() }))
	}
	t.cursor = ui.NewListCursor(len(t.items), index)
	t.cursor.Resize(t.height)
}

func (t table) current() (todos.Item, bool) {
	if len(t.items) == 0 {
		return todos.Item{}, false
	}
	return t.items[t.cursor.Index], true
}

func (t *table) resize(width, height int) {
	t.width = width
	t.height = max(height, 1)
	t.cursor.Resize(t.height)
}

func (t table) mine() int {
	n := 0
	for _, i := range t.all {
		if i.Mine {
			n++
		}
	}
	return n
}

func (t table) repos() int {
	dirs := map[string]bool{}
	for _, i := range t.all {
		dirs[i.Dir] = true
	}
	return len(dirs)
}

type layout struct {
	age, project, file, author, note int
}

// layout fits the columns to their content and gives the note what is left. When that is too
// little the author goes first, then the file narrows.
func (t table) layout() layout {
	l := layout{
		age:     t.fit("AGE", func(i todos.Item) ui.Cell { return ageCell(i, t.now) }, 0, t.width),
		project: t.fit("PROJECT", projectCell, minProjectWidth, maxProjectWidth),
		file:    t.fit("FILE", func(i todos.Item) ui.Cell { return ui.Cell{ui.NewSpan(i.Location(), plain)} }, minFileWidth, maxFileWidth),
		author:  t.fit("AUTHOR", authorCell, 0, maxAuthorWidth),
	}
	l.note = t.width - cursorWidth - l.age - markerWidth - l.project - l.file - l.author
	if l.note < minNoteWidth {
		l.note += l.author
		l.author = 0
	}
	if l.note < minNoteWidth {
		shrink := min(minNoteWidth-l.note, l.file-minFileWidth)
		l.file -= shrink
		l.note += shrink
	}
	return l
}

func (t table) fit(title string, field func(todos.Item) ui.Cell, least, most int) int {
	longest := lipgloss.Width(title)
	for _, i := range t.items {
		longest = max(longest, field(i).Width())
	}
	return min(most, max(least, longest+columnGap))
}

func (t table) header() string {
	l := t.layout()
	cells := strings.Repeat(" ", cursorWidth) + ui.PadRight("AGE", l.age) + ui.PadRight("", markerWidth) +
		ui.PadRight("NOTE", l.note) + ui.PadRight("PROJECT", l.project) + ui.PadRight("FILE", l.file)
	if l.author > 0 {
		cells += "AUTHOR"
	}
	return ui.Muted.Render(cells)
}

// view renders the visible rows, padded to the table's height when it scrolls, so what follows
// stays in place.
func (t table) view() string {
	l := t.layout()
	start, end := t.cursor.Visible()
	lines := make([]string, 0, t.height)
	for i := start; i < end; i++ {
		lines = append(lines, t.row(i, l))
	}
	for len(lines) < min(t.height, len(t.items)) {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (t table) row(index int, l layout) string {
	item := t.items[index]
	painter := ui.NewRowPainter(index == t.cursor.Index, t.highlight)
	row := painter.Cursor() +
		ageCell(item, t.now).Render(painter, l.age) +
		markerCell(item).Render(painter, markerWidth) +
		noteCell(item).Truncate(l.note-columnGap).Render(painter, l.note) +
		projectCell(item).Truncate(l.project-columnGap).Render(painter, l.project) +
		fileCell(item, l.file-columnGap).Render(painter, l.file)
	if l.author > 0 {
		row += authorCell(item).Truncate(l.author-columnGap).Render(painter, l.author)
	}
	return painter.Fill(row, t.width)
}
