package ui

import (
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"
)

// RowPainter renders the segments of one list row. For the cursor row it adds a background to
// every segment, since each segment's reset would otherwise end the background early.
type RowPainter struct {
	highlighted bool
	highlight   lipgloss.Style
}

// NewRowPainter returns a painter that highlights the row with background when highlighted is true.
func NewRowPainter(highlighted bool, background color.Color) RowPainter {
	if !highlighted {
		return RowPainter{}
	}
	return RowPainter{highlighted: true, highlight: lipgloss.NewStyle().Background(background)}
}

// IsHighlighted reports whether this is the cursor row.
func (p RowPainter) IsHighlighted() bool {
	return p.highlighted
}

// Paint renders text with style, on the highlight background for the cursor row.
func (p RowPainter) Paint(style lipgloss.Style, text string) string {
	if !p.highlighted {
		return style.Render(text)
	}
	return style.Inherit(p.highlight).Render(text)
}

// Fill extends the highlight to the full row width.
func (p RowPainter) Fill(row string, width int) string {
	if !p.highlighted {
		return row
	}
	return row + p.highlight.Render(strings.Repeat(" ", max(0, width-lipgloss.Width(row))))
}

// Cursor is the marker at the start of a row.
func (p RowPainter) Cursor() string {
	if !p.highlighted {
		return "  "
	}
	return p.Paint(Selected, "▸ ")
}
