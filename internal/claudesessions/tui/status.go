package tui

import (
	"strconv"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	openMark   = "● "
	idleMark   = "○ "
	statusHead = "STATUS"
)

var activityStyles = map[claudesessions.Activity]lipgloss.Style{
	claudesessions.Working: lipgloss.NewStyle().Foreground(ui.ColorAccent2),
	claudesessions.Waiting: ui.Warning,
	claudesessions.Idle:    ui.Muted,
}

// entrypointNames says where a Claude Code was started from, for the ones worth naming.
var entrypointNames = map[string]string{
	"cli":            "a terminal",
	"claude-desktop": "the Claude app",
	"claude-vscode":  "VS Code",
}

// statusCell is the status column's text: blank for a closed session, so the open ones stand out.
func statusCell(live claudesessions.Live) string {
	switch live.Activity {
	case claudesessions.Closed:
		return ""
	case claudesessions.Idle:
		return idleMark + live.Activity.String()
	default:
		return openMark + live.Activity.String()
	}
}

func statusStyle(live claudesessions.Live) lipgloss.Style {
	return activityStyles[live.Activity]
}

// liveSummary says what Claude is doing with an open session, for how long and where, such as
// "working for 2m in the Claude app (pid 13507)", or "" for a closed one.
func liveSummary(live claudesessions.Live, now time.Time) string {
	if live.Activity == claudesessions.Closed {
		return ""
	}
	summary := live.Activity.String()
	if live.Activity == claudesessions.Waiting && live.WaitingFor != "" {
		summary += " on " + live.WaitingFor
	}
	if !live.Since.IsZero() {
		summary += " for " + ui.Elapsed(max(now.Sub(live.Since), time.Second))
	}
	if where, ok := entrypointNames[live.Entrypoint]; ok {
		summary += " in " + where
	}
	return summary + " (pid " + strconv.Itoa(live.PID) + ")"
}
