package ui

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// HorizontalMargin is the gap kept between full-screen views and the terminal's side edges.
const HorizontalMargin = 1

// Screen frames a full-screen view with a blank line above it and side margins.
var Screen = lipgloss.NewStyle().Margin(1, HorizontalMargin, 0, HorizontalMargin)

// Cursor row backgrounds: a barely visible gray on either kind of terminal background.
var (
	highlightOnLight = lipgloss.Color("#EDEDF2")
	highlightOnDark  = lipgloss.Color("#2C2C36")
)

// HighlightColor is the background of the row under the cursor for a dark or light terminal.
func HighlightColor(darkBackground bool) color.Color {
	return lipgloss.LightDark(darkBackground)(highlightOnLight, highlightOnDark)
}
