// Package ui holds styling and widgets shared by ngt's terminal interfaces.
package ui

import "charm.land/lipgloss/v2"

var (
	ColorAccent  = lipgloss.Color("#7D56F4")
	ColorAccent2 = lipgloss.Color("#EE6FF8")
	ColorSuccess = lipgloss.Color("#04B575")
	ColorWarning = lipgloss.Color("#FFB000")
	ColorDanger  = lipgloss.Color("#FF4672")
	ColorMuted   = lipgloss.Color("#7A7A7A")
	ColorText    = lipgloss.Color("#DDDDDD")
)

var (
	Title    = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent2)
	Heading  = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	Muted    = lipgloss.NewStyle().Foreground(ColorMuted)
	Success  = lipgloss.NewStyle().Foreground(ColorSuccess)
	Warning  = lipgloss.NewStyle().Foreground(ColorWarning)
	Danger   = lipgloss.NewStyle().Foreground(ColorDanger)
	Bold     = lipgloss.NewStyle().Bold(true)
	Selected = lipgloss.NewStyle().Foreground(ColorAccent2).Bold(true)
	Box      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorAccent).Padding(1, 2)
	WarnBox  = Box.BorderForeground(ColorDanger)
	Help     = lipgloss.NewStyle().Foreground(ColorMuted).MarginTop(1)
)
