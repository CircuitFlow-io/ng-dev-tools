// Package tui is the terminal interface for `ngt doctor`: a progress screen while checks
// run, and the report printed once they finish.
package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/doctor"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

type (
	progressMsg doctor.Progress
	doneMsg     []doctor.Outcome
)

// Model shows progress while the doctor runs. The report itself is printed by the caller after
// the program exits, so it can be taller than the terminal and stays in the scrollback.
type Model struct {
	ctx       context.Context
	cancel    context.CancelFunc
	doctor    doctor.Doctor
	events    chan tea.Msg
	panel     ui.ProgressPanel
	outcomes  []doctor.Outcome
	finished  bool
	cancelled bool
}

// New creates the model. Cancelling ctx stops the checks.
func New(ctx context.Context, d doctor.Doctor) Model {
	ctx, cancel := context.WithCancel(ctx)
	panel := ui.NewProgressPanel("Checking your Mac")
	panel.SetStatus(progressStatus(doctor.Progress{Total: len(d.Checks)}), "")
	return Model{
		ctx:    ctx,
		cancel: cancel,
		doctor: d,
		events: make(chan tea.Msg, ui.EventBuffer),
		panel:  panel,
	}
}

// Outcomes are the results of every check, once they have all finished.
func (m Model) Outcomes() []doctor.Outcome {
	return m.outcomes
}

// Cancelled reports whether the user quit before the checks finished.
func (m Model) Cancelled() bool {
	return m.cancelled
}

// Init starts the checks.
func (m Model) Init() tea.Cmd {
	d, ctx := m.doctor, m.ctx
	run := ui.RunInBackground(m.events, func(notify func(tea.Msg)) tea.Msg {
		return doneMsg(d.Run(ctx, func(p doctor.Progress) { notify(progressMsg(p)) }))
	})
	return tea.Batch(m.panel.Init(), run)
}

// Update tracks progress and quits when the checks finish or the user cancels.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.panel.SetWidth(msg.Width)
		return m, nil
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case progressMsg:
		p := doctor.Progress(msg)
		m.panel.SetStatus(progressStatus(p), p.Current)
		return m, tea.Batch(m.panel.SetPercent(float64(p.Done)/float64(max(p.Total, 1))), ui.WaitForEvent(m.events))
	case doneMsg:
		m.outcomes = msg
		m.finished = true
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.panel, cmd = m.panel.Update(msg)
	return m, cmd
}

func (m Model) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c", "q", "esc":
		m.cancel()
		m.cancelled = true
		return m, tea.Quit
	}
	return m, nil
}

// View shows the progress panel until the program exits.
func (m Model) View() tea.View {
	if m.finished || m.cancelled {
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(m.panel.View()))
	v.WindowTitle = "ngt doctor"
	v.AltScreen = true
	return v
}

func progressStatus(p doctor.Progress) string {
	return fmt.Sprintf("%s  %d/%d checks", ui.Heading.Render("Checking"), p.Done, p.Total)
}
