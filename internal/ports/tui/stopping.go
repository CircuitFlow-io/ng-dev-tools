package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

type (
	stopProgressMsg ports.StopProgress
	stopDoneMsg     []ports.StopResult
)

func (m Model) startStopping() (tea.Model, tea.Cmd) {
	m.state = stateStopping
	m.panel = ui.NewProgressPanel("Stopping processes")
	m.panel.SetWidth(m.width)
	m.panel.SetStatus(stopStatus(ports.StopProgress{Total: len(m.chosen)}), "")

	stopper, processes, ctx := m.cfg.Stopper, m.chosen, m.ctx
	stop := ui.RunInBackground(m.events, func(notify func(tea.Msg)) tea.Msg {
		return stopDoneMsg(stopper.StopAll(ctx, processes, func(p ports.StopProgress) { notify(stopProgressMsg(p)) }))
	})
	return m, tea.Batch(m.panel.Init(), stop)
}

func (m Model) updateStopping(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case stopProgressMsg:
		p := ports.StopProgress(msg)
		m.panel.SetStatus(stopStatus(p), p.Last)
		return m, tea.Batch(m.panel.SetPercent(float64(p.Done)/float64(p.Total)), ui.WaitForEvent(m.events))
	case stopDoneMsg:
		m.results = msg
		return m.finish(false)
	}
	var cmd tea.Cmd
	m.panel, cmd = m.panel.Update(msg)
	return m, cmd
}

func stopStatus(p ports.StopProgress) string {
	return fmt.Sprintf("%s  %d/%d processes", ui.Heading.Render("Stopping"), p.Done, p.Total)
}
