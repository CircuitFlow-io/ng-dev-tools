package tui

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const selectingHelp = "↑/↓ move · space toggle · a all · n none · enter stop · r refresh · q quit"

func (m Model) updateSelecting(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.list.cursor.HandleKey(key.String()) {
		return m, nil
	}
	switch key.String() {
	case "space", "x":
		m.list.toggle()
	case "a":
		m.list.setAll(true)
	case "n":
		m.list.setAll(false)
	case "r":
		return m.refresh()
	case "enter":
		return m.confirmSelection()
	case "q", "esc":
		return m.finish(true)
	}
	return m, nil
}

// confirmSelection stops the checked processes, or the one under the cursor when none is checked.
func (m Model) confirmSelection() (tea.Model, tea.Cmd) {
	m.chosen = m.list.selectedProcesses()
	if len(m.chosen) == 0 {
		m.chosen = []ports.Process{m.list.current()}
	}
	m.state = stateConfirming
	return m, nil
}

func (m Model) selectingView() string {
	lines := []string{
		m.listTitle(),
		"",
		m.list.header(),
		m.list.view(time.Now()),
		"",
	}
	lines = append(lines, m.currentDetails()...)
	lines = append(lines, m.selectionSummary(), ui.Help.Render(selectingHelp))
	return strings.Join(lines, "\n")
}

func (m Model) listTitle() string {
	count := len(m.list.processes)
	title := ui.Title.Render(ui.Count(count, "process") + " listening")
	if m.hidden == 0 {
		return title
	}
	return title + ui.Muted.Render(fmt.Sprintf("  (%s hidden, --all shows them)", ui.Count(m.hidden, "macOS system process")))
}

// currentDetails always returns three lines so the layout does not jump while moving.
func (m Model) currentDetails() []string {
	p := m.list.current()
	width := m.width - 2*ui.HorizontalMargin
	sockets := fmt.Sprintf("PID %d · %s", p.PID, strings.Join(p.Addresses, ", "))
	if p.IsExposed() {
		sockets += ui.Warning.Render(" · reachable from other devices on your network")
	}
	return []string{
		ui.Muted.Render(ui.Truncate(p.CommandLine, width)),
		ui.Muted.Render(sockets),
		ui.Warning.Render(ui.Truncate(p.Caution(), width)),
	}
}

func (m Model) selectionSummary() string {
	count := len(m.list.selectedProcesses())
	if count == 0 {
		return ui.Muted.Render("Nothing checked: enter stops the process under the cursor")
	}
	return ui.Bold.Render(fmt.Sprintf("%d selected", count))
}
