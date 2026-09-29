package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

type (
	cleanProgressMsg cleanup.CleanProgress
	cleanDoneMsg     []cleanup.CleanResult
)

func (m Model) startCleaning() (tea.Model, tea.Cmd) {
	m.state = stateCleaning
	title := "Cleaning up"
	if m.cfg.Cleaner.DryRun {
		title = "Dry run: simulating cleanup"
	}
	m.panel = ui.NewProgressPanel(title)
	m.panel.SetWidth(m.width)
	m.panel.SetStatus(cleanStatus(cleanup.CleanProgress{Total: len(m.chosen), BytesTotal: cleanup.TotalSize(m.chosen)}), "")

	cleaner, items, ctx := m.cfg.Cleaner, m.chosen, m.ctx
	clean := ui.RunInBackground(m.events, func(notify func(tea.Msg)) tea.Msg {
		return cleanDoneMsg(cleaner.Clean(ctx, items, func(p cleanup.CleanProgress) { notify(cleanProgressMsg(p)) }))
	})
	return m, tea.Batch(m.panel.Init(), clean)
}

func (m Model) updateCleaning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case cleanProgressMsg:
		p := cleanup.CleanProgress(msg)
		if !m.stopping {
			m.panel.SetStatus(cleanStatus(p), p.Current)
		}
		return m, tea.Batch(m.panel.SetPercent(cleanPercent(p)), ui.WaitForEvent(m.events))
	case cleanDoneMsg:
		m.results = msg
		return m.finish(m.stopping)
	}
	var cmd tea.Cmd
	m.panel, cmd = m.panel.Update(msg)
	return m, cmd
}

// cleanPercent weights progress by bytes so the bar tracks space freed, not item count.
func cleanPercent(p cleanup.CleanProgress) float64 {
	if p.BytesTotal > 0 {
		return float64(p.BytesDone) / float64(p.BytesTotal)
	}
	if p.Total > 0 {
		return float64(p.Done) / float64(p.Total)
	}
	return 1
}

func cleanStatus(p cleanup.CleanProgress) string {
	return fmt.Sprintf("%s  %d/%d items · %s of %s",
		ui.Heading.Render("Deleting"), p.Done, p.Total,
		ui.Success.Render(ui.Bytes(p.BytesDone)), ui.Bytes(p.BytesTotal))
}
