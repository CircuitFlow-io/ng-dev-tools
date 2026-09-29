package tui

import (
	"image/color"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth     = 2
	checkboxWidth   = 4
	portsWidth      = 14
	nameWidth       = 20
	uptimeWidth     = 9
	tagWidth        = 10
	minProjectWidth = 12
)

// processList is a scrollable multi-select list of listening processes.
type processList struct {
	processes []ports.Process
	selected  []bool
	cursor    ui.ListCursor
	width     int
	home      string
	highlight color.Color
}

// newProcessList keeps processes selected that were selected before a refresh.
func newProcessList(processes []ports.Process, keepSelected map[int]bool, home string) processList {
	l := processList{
		processes: processes,
		selected:  make([]bool, len(processes)),
		cursor:    ui.NewListCursor(len(processes), 0),
		home:      home,
	}
	for i, p := range processes {
		l.selected[i] = keepSelected[p.PID]
	}
	return l
}

func (l *processList) resize(width, height int) {
	l.width = width
	l.cursor.Resize(height)
}

func (l *processList) toggle() {
	l.selected[l.cursor.Index] = !l.selected[l.cursor.Index]
}

func (l *processList) setAll(selected bool) {
	for i := range l.selected {
		l.selected[i] = selected
	}
}

func (l processList) current() ports.Process {
	return l.processes[l.cursor.Index]
}

func (l processList) selectedProcesses() []ports.Process {
	var chosen []ports.Process
	for i, p := range l.processes {
		if l.selected[i] {
			chosen = append(chosen, p)
		}
	}
	return chosen
}

func (l processList) selectedPIDs() map[int]bool {
	pids := map[int]bool{}
	for _, p := range l.selectedProcesses() {
		pids[p.PID] = true
	}
	return pids
}

func (l processList) projectWidth() int {
	return max(minProjectWidth, l.width-cursorWidth-checkboxWidth-portsWidth-nameWidth-uptimeWidth-tagWidth)
}

func (l processList) header() string {
	cells := strings.Repeat(" ", cursorWidth+checkboxWidth) +
		ui.PadRight("PORT", portsWidth) + ui.PadRight("PROCESS", nameWidth) +
		ui.PadRight("PROJECT", l.projectWidth()) + ui.PadLeft("UPTIME", uptimeWidth)
	return ui.Muted.Render(cells)
}

func (l processList) view(now time.Time) string {
	start, end := l.cursor.Visible()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, l.renderRow(i, now))
	}
	return strings.Join(lines, "\n")
}

func (l processList) renderRow(index int, now time.Time) string {
	p := l.processes[index]
	painter := ui.NewRowPainter(index == l.cursor.Index, l.highlight)

	box := painter.Paint(ui.Muted, "[ ] ")
	if l.selected[index] {
		box = painter.Paint(ui.Success, "[✓] ")
	}
	nameStyle := lipgloss.NewStyle().Width(nameWidth)
	if painter.IsHighlighted() {
		nameStyle = nameStyle.Bold(true)
	}
	projectWidth := l.projectWidth()

	row := painter.Cursor() + box +
		painter.Paint(ui.Heading.Width(portsWidth), ui.Truncate(p.PortList(), portsWidth-1)) +
		painter.Paint(nameStyle, ui.Truncate(p.Name, nameWidth-1)) +
		painter.Paint(ui.Muted.Width(projectWidth), ui.TruncatePath(ui.TildePath(p.Project, l.home), projectWidth-1)) +
		painter.Paint(lipgloss.NewStyle().Width(uptimeWidth).Align(lipgloss.Right), ui.Elapsed(p.Uptime(now))) +
		painter.Paint(lipgloss.NewStyle(), "  ") +
		painter.Paint(ui.Warning, p.Tag())
	return painter.Fill(row, l.width)
}
