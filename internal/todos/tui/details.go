package tui

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// detailLines is the fixed height of the details box's content, so the table does not jump.
	detailLines      = 8
	detailFrameWidth = 4
	// headerLines are the location and blame lines above the code.
	headerLines   = 2
	linesAbove    = 2
	linesBelow    = detailLines - headerLines - linesAbove - 1
	codeSeparator = " │ "
	tabWidth      = 4
)

var (
	detailBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.ColorAccent).Padding(0, 1)
	markedLine = lipgloss.NewStyle().Bold(true)
)

// details shows where the item under the cursor is, who left it and when, and the code around it.
func details(item todos.Item, ok bool, code []todos.SourceLine, codeErr error, now time.Time, width int) string {
	inner := width - detailFrameWidth
	if !ok {
		return detailBox.Width(width).Render(padLines([]string{ui.Success.Render("Nothing to do: no TODO, FIXME or HACK comments here")}))
	}
	lines := []string{ui.FitLine(locationLine(item), inner), ui.FitLine(blameLine(item, now), inner)}
	if codeErr != nil {
		lines = append(lines, ui.FitLine(ui.Warning.Render("Could not read the file: "+codeErr.Error()), inner))
	}
	lines = append(lines, codeLines(item, code, inner)...)
	return detailBox.Width(width).Render(padLines(lines[:min(len(lines), detailLines)]))
}

func locationLine(item todos.Item) string {
	return markerCell(item).String() + "  " + ui.Bold.Render(item.Project) + "  " + ui.Muted.Render(item.Location())
}

func blameLine(item todos.Item, now time.Time) string {
	if item.Uncommitted {
		return ui.Muted.Render("Not committed yet")
	}
	who := item.Author
	if item.Mine {
		who = you
	}
	line := ui.Muted.Render("Added by ") + who + ui.Muted.Render(" "+ui.Ago(now, item.At)+" in ") + accent.Render(item.ShortCommit())
	return line + ui.Muted.Render(noteJoiner+item.Subject)
}

// codeLines numbers the lines around the marker and brings out the marker's line.
func codeLines(item todos.Item, code []todos.SourceLine, width int) []string {
	numberWidth := len(strconv.Itoa(item.Line + linesBelow))
	var lines []string
	for _, l := range code {
		number := ui.PadLeft(strconv.Itoa(l.Number), numberWidth)
		text := strings.ReplaceAll(l.Text, "\t", strings.Repeat(" ", tabWidth))
		if l.Number == item.Line {
			lines = append(lines, ui.FitLine(markerStyles[item.Marker].Render(number)+ui.Muted.Render(codeSeparator)+ui.RenderTickets(text, markedLine), width))
			continue
		}
		lines = append(lines, ui.FitLine(ui.Muted.Render(number+codeSeparator+text), width))
	}
	return lines
}

func padLines(lines []string) string {
	for len(lines) < detailLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
