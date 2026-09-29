package tui

import (
	"image/color"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth    = 2
	glyphWidth     = 2
	columnGap      = 2
	minFolderWidth = 12
	maxFolderWidth = 40
	minFilesWidth  = 8
	maxFilesWidth  = 26
	minNotesWidth  = 12
)

// table is the list of env sets with a cursor.
type table struct {
	sets      []envfiles.Set
	cursor    ui.ListCursor
	width     int
	height    int
	highlight color.Color
}

func newTable() table {
	return table{height: 1}
}

// setSets replaces the rows, most urgent first, keeping the cursor on the same set.
func (t *table) setSets(sets []envfiles.Set) {
	current, hadCurrent := t.current()
	envfiles.Sort(sets)
	t.sets = sets
	index := 0
	if hadCurrent {
		index = max(0, slices.IndexFunc(sets, func(s envfiles.Set) bool { return s.ID() == current.ID() }))
	}
	t.cursor = ui.NewListCursor(len(sets), index)
	t.cursor.Resize(t.height)
}

func (t table) current() (envfiles.Set, bool) {
	if len(t.sets) == 0 {
		return envfiles.Set{}, false
	}
	return t.sets[t.cursor.Index], true
}

func (t *table) resize(width, height int) {
	t.width = width
	t.height = max(height, 1)
	t.cursor.Resize(t.height)
}

func (t table) needingAttention() int {
	n := 0
	for _, s := range t.sets {
		if s.Attention().NeedsAttention() {
			n++
		}
	}
	return n
}

func (t table) folders() int {
	dirs := map[string]bool{}
	for _, s := range t.sets {
		dirs[s.Dir] = true
	}
	return len(dirs)
}

type layout struct {
	folder, files, missing, extra, empty, notes int
}

// layout fits the columns to their content. The notes take what is left, and are left out when
// that is too little: the glyph and the details box still tell what they would.
func (t table) layout() layout {
	l := layout{
		files:   t.fit("FILES", filesCell, minFilesWidth, maxFilesWidth),
		missing: t.fit("MISSING", missingCell, 0, t.width),
		extra:   t.fit("EXTRA", extraCell, 0, t.width),
		empty:   t.fit("EMPTY", emptyCell, 0, t.width),
	}
	folder := t.fit("FOLDER", func(s envfiles.Set) ui.Cell { return folderCell(s, plain) }, minFolderWidth, maxFolderWidth)
	rest := t.width - cursorWidth - glyphWidth - l.files - l.missing - l.extra - l.empty
	if rest-folder < minNotesWidth {
		l.folder = max(minFolderWidth, rest)
		return l
	}
	l.folder = folder
	l.notes = rest - folder
	return l
}

func (t table) fit(title string, field func(envfiles.Set) ui.Cell, least, most int) int {
	longest := lipgloss.Width(title)
	for _, s := range t.sets {
		longest = max(longest, field(s).Width())
	}
	return min(most, max(least, longest+columnGap))
}

func (t table) header() string {
	l := t.layout()
	cells := strings.Repeat(" ", cursorWidth+glyphWidth) +
		ui.PadRight("FOLDER", l.folder) + ui.PadRight("FILES", l.files) + ui.PadRight("MISSING", l.missing) +
		ui.PadRight("EXTRA", l.extra) + ui.PadRight("EMPTY", l.empty)
	if l.notes > 0 {
		cells += "NOTES"
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
	for len(lines) < min(t.height, len(t.sets)) {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (t table) row(index int, l layout) string {
	s := t.sets[index]
	painter := ui.NewRowPainter(index == t.cursor.Index, t.highlight)
	nameStyle := plain
	if painter.IsHighlighted() {
		nameStyle = ui.Bold
	}
	row := painter.Cursor() +
		glyphCell(s).Render(painter, glyphWidth) +
		folderCell(s, nameStyle).Truncate(l.folder-columnGap).Render(painter, l.folder) +
		filesCell(s).Truncate(l.files-columnGap).Render(painter, l.files) +
		missingCell(s).Render(painter, l.missing) +
		extraCell(s).Render(painter, l.extra) +
		emptyCell(s).Render(painter, l.empty)
	if l.notes > 0 {
		row += notesCell(s).Render(painter, l.notes)
	}
	return painter.Fill(row, t.width)
}
