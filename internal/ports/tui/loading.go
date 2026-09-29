package tui

import (
	"slices"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/ports"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

type listedMsg struct {
	processes []ports.Process
	err       error
}

func (m Model) startListing() tea.Cmd {
	lister, ctx := m.cfg.Lister, m.ctx
	return ui.RunInBackground(m.events, func(func(tea.Msg)) tea.Msg {
		processes, err := lister.List(ctx)
		return listedMsg{processes: processes, err: err}
	})
}

// refresh lists again, keeping the current selection for processes that are still there.
func (m Model) refresh() (tea.Model, tea.Cmd) {
	m.state = stateLoading
	return m, tea.Batch(m.spinner.Tick, m.startListing())
}

func (m Model) updateLoading(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case listedMsg:
		return m.listed(msg)
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "q" || msg.String() == "esc" {
			return m.interrupt()
		}
	}
	return m, nil
}

func (m Model) listed(msg listedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m.finish(false)
	}
	processes := msg.processes
	m.hidden = 0
	if !m.cfg.ShowSystem {
		processes = slices.DeleteFunc(processes, ports.Process.IsSystem)
		m.hidden = len(msg.processes) - len(processes)
	}
	if len(processes) == 0 {
		return m.finish(false)
	}
	m.list = newProcessList(processes, m.list.selectedPIDs(), m.cfg.Lister.Home)
	m.list.highlight = ui.HighlightColor(m.darkBG)
	m.resize(m.width, m.height)
	m.state = stateSelecting
	return m, nil
}
