package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

var plain = lipgloss.NewStyle()

// Span is a run of text in one style.
type Span struct {
	text  string
	style lipgloss.Style
}

// NewSpan styles text.
func NewSpan(text string, style lipgloss.Style) Span {
	return Span{text: text, style: style}
}

// Cell is the styled spans of one table cell, such as "+2 ~3" in two colours.
type Cell []Span

// Width is the cell's width in terminal columns.
func (c Cell) Width() int {
	return lipgloss.Width(c.Text())
}

// Text is the cell without styling.
func (c Cell) Text() string {
	var b strings.Builder
	for _, s := range c {
		b.WriteString(s.text)
	}
	return b.String()
}

// String renders the cell's spans without padding.
func (c Cell) String() string {
	var b strings.Builder
	for _, s := range c {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

// Truncate cuts the cell to width columns, keeping each span's style and ending with … when cut.
func (c Cell) Truncate(width int) Cell {
	if c.Width() <= width {
		return c
	}
	var cut Cell
	room := width
	for _, s := range c {
		if room <= 0 {
			break
		}
		text := Truncate(s.text, room)
		room -= lipgloss.Width(text)
		cut = append(cut, Span{text: text, style: s.style})
	}
	return cut
}

// Render paints the cell for a table row, padded to width and cut short when it does not fit.
func (c Cell) Render(painter RowPainter, width int) string {
	cut := c.Truncate(width)
	var b strings.Builder
	for _, s := range cut {
		b.WriteString(painter.Paint(s.style, s.text))
	}
	if room := width - cut.Width(); room > 0 {
		b.WriteString(painter.Paint(plain, strings.Repeat(" ", room)))
	}
	return b.String()
}

// JoinCells puts cells one after another with a muted joiner between them.
func JoinCells(cells []Cell, joiner string) Cell {
	var joined Cell
	for i, c := range cells {
		if i > 0 {
			joined = append(joined, NewSpan(joiner, Muted))
		}
		joined = append(joined, c...)
	}
	return joined
}
