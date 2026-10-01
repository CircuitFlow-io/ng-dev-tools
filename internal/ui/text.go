package ui

import (
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

const (
	ellipsis = "…"
	// maxBytesPerCell bounds how much of a long string is measured to fill a width: more than any
	// grapheme takes per cell in practice, so a pasted megabyte costs no more than a line of it.
	maxBytesPerCell = 32
	escape          = "\x1b"
)

// Truncate shortens s to at most width cells, keeping the start.
func Truncate(s string, width int) string {
	if width <= 0 {
		return s
	}
	head, cut := head(s, width*maxBytesPerCell)
	if !cut {
		return ansi.Truncate(s, width, ellipsis)
	}
	return ansi.Truncate(head, width-lipgloss.Width(ellipsis), "") + ellipsis
}

// head is the start of s up to limit bytes, cut at a character, and whether anything was left out.
// Text with escape codes is kept whole, since a cut could break one.
func head(s string, limit int) (string, bool) {
	if len(s) <= limit || strings.Contains(s, escape) {
		return s, false
	}
	for limit > 0 && !utf8.RuneStart(s[limit]) {
		limit--
	}
	return s[:limit], true
}

// TruncatePath shortens path to at most width cells, keeping the end, which is its informative part.
func TruncatePath(path string, width int) string {
	pathWidth := lipgloss.Width(path)
	if width <= 0 || pathWidth <= width {
		return path
	}
	room := width - lipgloss.Width(ellipsis)
	kept := ansi.TruncateLeft(path, pathWidth-room, "")
	for lipgloss.Width(kept) > room {
		// TruncateLeft keeps a wide character the cut falls inside.
		_, size := utf8.DecodeRuneInString(kept)
		kept = kept[size:]
	}
	return ellipsis + kept
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
