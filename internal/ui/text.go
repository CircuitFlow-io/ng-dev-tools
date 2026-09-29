package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const ellipsis = "…"

// Truncate shortens s to at most width cells, keeping the start.
func Truncate(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + ellipsis
}

// TruncatePath shortens path to at most width cells, keeping the end, which is its informative part.
func TruncatePath(path string, width int) string {
	if width <= 0 || lipgloss.Width(path) <= width {
		return path
	}
	runes := []rune(path)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		runes = runes[1:]
	}
	return ellipsis + string(runes)
}

// PadRight pads s with spaces to width cells.
func PadRight(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Render(s)
}

// PadLeft right-aligns s within width cells.
func PadLeft(s string, width int) string {
	return lipgloss.NewStyle().Width(width).Align(lipgloss.Right).Render(s)
}

// TildePath shows paths inside home as "~/...".
func TildePath(path, home string) string {
	if home == "" {
		return path
	}
	if path == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(path, home+"/"); ok {
		return "~/" + rest
	}
	return path
}
