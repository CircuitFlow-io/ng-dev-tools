package ui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

const shimmerSteps = 24

// shimmerPalette runs accent -> pink -> accent so the cycle loops without a seam.
var shimmerPalette = lipgloss.Blend1D(shimmerSteps, ColorAccent, ColorAccent2, ColorAccent)

// Shimmer renders text with a gradient that moves one step per frame.
func Shimmer(text string, frame int) string {
	var b strings.Builder
	for i, r := range []rune(text) {
		c := shimmerPalette[(frame+len(shimmerPalette)-i%len(shimmerPalette))%len(shimmerPalette)]
		b.WriteString(lipgloss.NewStyle().Bold(true).Foreground(c).Render(string(r)))
	}
	return b.String()
}
