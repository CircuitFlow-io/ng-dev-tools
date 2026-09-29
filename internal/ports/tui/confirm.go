package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

func (m Model) updateConfirming(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "y", "Y", "enter":
		return m.startStopping()
	case "n", "N", "esc", "q":
		m.state = stateSelecting
	}
	return m, nil
}

// boxChrome is the width the confirmation box's border and padding take.
const boxChrome = 6

func (m Model) confirmView() string {
	width := m.width - 2*ui.HorizontalMargin - boxChrome
	lines := []string{ui.Danger.Bold(true).Render(fmt.Sprintf("Stop %s?", ui.Count(len(m.chosen), "process"))), ""}
	for _, p := range m.chosen {
		lines = append(lines, chosenLine(p, m.cfg.Lister.Home, width))
		if caution := p.Caution(); caution != "" {
			lines = append(lines, ui.Warning.Render(ui.Truncate("    "+caution, width)))
		}
	}
	lines = append(lines, "", StopMethod(m.cfg.Stopper), "", ui.Bold.Render("y")+" confirm   "+ui.Bold.Render("n")+" back to the list")
	return ui.WarnBox.Render(strings.Join(lines, "\n"))
}

func chosenLine(p ports.Process, home string, width int) string {
	label := "  • " + p.Label()
	project := ui.TruncatePath(ui.TildePath(p.Project, home), width-lipgloss.Width(label)-2)
	return label + ui.Muted.Render("  "+project)
}

// StopMethod describes how processes will be ended, for confirmation prompts.
func StopMethod(s ports.Stopper) string {
	if s.Force {
		return "They are killed immediately with SIGKILL, without a chance to clean up."
	}
	grace := s.Grace
	if grace <= 0 {
		grace = ports.DefaultGrace
	}
	return fmt.Sprintf("They get SIGTERM to shut down cleanly and SIGKILL if still running after %s.", grace)
}
