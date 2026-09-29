package tui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// discoveryShare is the part of the scan progress bar given to running rules; measuring
	// sizes takes the rest because it does most of the disk I/O.
	discoveryShare = 0.3
	// maxUnfinishedPercent keeps the bar from reading 100% while the last item is still measured.
	maxUnfinishedPercent = 0.99
)

type (
	scanProgressMsg cleanup.ScanProgress
	scanDoneMsg     cleanup.ScanResult
)

func (m Model) startScan() tea.Cmd {
	return ui.RunInBackground(m.events, func(notify func(tea.Msg)) tea.Msg {
		result := m.cfg.Scanner.Scan(m.ctx, func(p cleanup.ScanProgress) { notify(scanProgressMsg(p)) })
		return scanDoneMsg(result)
	})
}

func (m Model) updateScanning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case scanProgressMsg:
		m.panel.SetStatus(scanStatus(cleanup.ScanProgress(msg)), msg.Current)
		return m, tea.Batch(m.panel.SetPercent(scanPercent(cleanup.ScanProgress(msg))), ui.WaitForEvent(m.events))
	case scanDoneMsg:
		return m.scanFinished(cleanup.ScanResult(msg))
	case tea.KeyPressMsg:
		if msg.String() == "q" || msg.String() == "esc" {
			return m.interrupt()
		}
	}
	var cmd tea.Cmd
	m.panel, cmd = m.panel.Update(msg)
	return m, cmd
}

func (m Model) scanFinished(result cleanup.ScanResult) (tea.Model, tea.Cmd) {
	m.warnings = result.Warnings
	if len(result.Items) == 0 {
		return m.finish(false)
	}
	m.list = newChecklist(result.Items)
	m.list.highlight = ui.HighlightColor(m.darkBG)
	m.resize(m.width, m.height)
	m.state = stateSelecting
	return m, nil
}

func scanPercent(p cleanup.ScanProgress) float64 {
	fraction := 1.0
	if p.Total > 0 {
		fraction = float64(p.Done) / float64(p.Total)
	}
	if p.Phase == cleanup.PhaseDiscovering {
		return discoveryShare * fraction
	}
	if p.Done < p.Total {
		return min(maxUnfinishedPercent, discoveryShare+(1-discoveryShare)*fraction)
	}
	return 1
}

func scanStatus(p cleanup.ScanProgress) string {
	if p.Phase == cleanup.PhaseDiscovering {
		return fmt.Sprintf("%s  %d/%d locations checked · %d candidates",
			ui.Heading.Render("Discovering"), p.Done, p.Total, p.ItemsFound)
	}
	return fmt.Sprintf("%s  %d/%d measured · %s reclaimable",
		ui.Heading.Render("Measuring"), p.Done, p.Total, ui.Bold.Render(ui.Bytes(p.BytesFound)))
}
