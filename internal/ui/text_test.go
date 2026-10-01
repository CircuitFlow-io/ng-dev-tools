package ui

import (
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
)

// cutRuneByRune is how Truncate and TruncatePath used to cut, one rune at a time, which is right
// but slow on long text; the new ones must give the same results.
func cutRuneByRune(s string, width int, fromStart bool) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes))+1 > width {
		if fromStart {
			runes = runes[:len(runes)-1]
		} else {
			runes = runes[1:]
		}
	}
	if fromStart {
		return string(runes) + ellipsis
	}
	return ellipsis + string(runes)
}

func TestTruncateMatchesCuttingRuneByRune(t *testing.T) {
	texts := []string{"", "short", "Upgrade the Expo SDK please", "日本語のテキストです", "emoji 🎉🎉 party", "~/projects/clients/acme/api"}
	for _, s := range texts {
		for width := range 30 {
			if got, want := Truncate(s, width), cutRuneByRune(s, width, true); got != want {
				t.Errorf("Truncate(%q, %d) = %q, want %q", s, width, got, want)
			}
			if got, want := TruncatePath(s, width), cutRuneByRune(s, width, false); got != want {
				t.Errorf("TruncatePath(%q, %d) = %q, want %q", s, width, got, want)
			}
		}
	}
}

func TestTruncateIsQuickOnAPastedMegabyte(t *testing.T) {
	pasted := strings.Repeat("ERROR something failed at module.function(line) ", 100_000)
	start := time.Now()
	got := Truncate(pasted, 80)
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("took %v", elapsed)
	}
	if want := cutRuneByRune(pasted[:200], 80, true); got != want {
		t.Errorf("Truncate = %q, want %q", got, want)
	}
}
