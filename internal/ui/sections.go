package ui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// minItemsForMoreLine is how many items a cut-short section needs before one of them gives way to
// an "… n more" line.
const minItemsForMoreLine = 2

// spreadGap is the least space Spread keeps between its two ends.
const spreadGap = 2

// Section is a titled list in a details box, such as a pull request's checks.
type Section struct {
	Title string
	Items []string
}

// FitSections shows each section's title and shares the lines left among their items in turn, so
// one long list does not hide the others. Sections that do not fit at all are left out.
func FitSections(sections []Section, width, height int) []string {
	for len(sections) > height {
		sections = sections[:len(sections)-1]
	}
	shown := make([]int, len(sections))
	for room, more := height-len(sections), true; room > 0 && more; {
		more = false
		for i, s := range sections {
			if room > 0 && shown[i] < len(s.Items) {
				shown[i]++
				room--
				more = true
			}
		}
	}
	var lines []string
	for i, s := range sections {
		lines = append(lines, FitLine(s.Title, width))
		for _, item := range visibleItems(s.Items, shown[i]) {
			lines = append(lines, FitLine(item, width))
		}
	}
	return lines
}

// visibleItems is the first n items, the last replaced by "… k more" when some are left out.
func visibleItems(items []string, n int) []string {
	if n >= len(items) || n < minItemsForMoreLine {
		return items[:n]
	}
	visible := append([]string(nil), items[:n-1]...)
	return append(visible, Muted.Render(fmt.Sprintf("%s %d more", ellipsis, len(items)-n+1)))
}

// FitLine cuts a styled line to width.
func FitLine(line string, width int) string {
	return ansi.Truncate(line, max(width, 0), ellipsis)
}

// SideBySide joins two columns of lines, the left one padded to leftWidth, with a muted separator.
func SideBySide(left, right []string, leftWidth int, separator string) []string {
	lines := make([]string, max(len(left), len(right)))
	for i := range lines {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines[i] = lipgloss.NewStyle().Width(leftWidth).Render(l) + Muted.Render(separator) + r
	}
	return lines
}

// Spread puts left at the start of a line and right at its end, cutting left short to keep right
// whole when they do not both fit.
func Spread(left, right string, width int) string {
	rightWidth := lipgloss.Width(right)
	if right == "" || rightWidth+spreadGap >= width {
		return FitLine(strings.TrimSpace(left+"  "+right), width)
	}
	left = FitLine(left, width-rightWidth-spreadGap)
	return left + strings.Repeat(" ", width-lipgloss.Width(left)-rightWidth) + right
}
