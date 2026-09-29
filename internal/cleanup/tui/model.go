// Package tui is the interactive terminal interface for `ngt clean`.
package tui

import (
	"context"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
	// chromeLines is the space taken by everything around the checklist on the selection screen.
	chromeLines = 10
)

type state int

const (
	stateScanning state = iota
	stateSelecting
	stateConfirming
	stateCleaning
	stateDone
)

// Config wires the interface to the scanner and cleaner.
type Config struct {
	Scanner cleanup.Scanner
	Cleaner cleanup.Cleaner
	LogPath string
}

// Model is the Bubble Tea model driving the whole clean flow.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config
	events chan tea.Msg
	now    time.Time

	state    state
	width    int
	height   int
	panel    ui.ProgressPanel
	list     checklist
	warnings []error
	chosen   []cleanup.Item
	results  []cleanup.CleanResult
	sudoErr  error
	aborted  bool
	stopping bool
	darkBG   bool
}

// New creates the model. Cancelling ctx stops any scan or clean in progress.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	return Model{
		ctx:    ctx,
		cancel: cancel,
		cfg:    cfg,
		events: make(chan tea.Msg, ui.EventBuffer),
		now:    cfg.Scanner.Env.Now,
		width:  defaultWidth,
		height: defaultHeight,
		darkBG: true,
		panel:  ui.NewProgressPanel("Scanning your Mac for reclaimable space"),
	}
}

// Init starts the scan.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.panel.Init(), m.startScan(), tea.RequestBackgroundColor)
}

// Update routes messages to the handler for the current screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.BackgroundColorMsg:
		m.darkBG = msg.IsDark()
		m.list.highlight = ui.HighlightColor(m.darkBG)
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.interrupt()
		}
	}

	switch m.state {
	case stateScanning:
		return m.updateScanning(msg)
	case stateSelecting:
		return m.updateSelecting(msg)
	case stateConfirming:
		return m.updateConfirming(msg)
	case stateCleaning:
		return m.updateCleaning(msg)
	default:
		return m, nil
	}
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateScanning, stateCleaning:
		content = m.panel.View()
	case stateSelecting:
		content = m.selectingView()
	case stateConfirming:
		content = m.confirmView()
	case stateDone:
		// The renderer clears the frame's last line on exit, so end the summary with a blank one.
		content = m.summaryView() + "\n"
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt clean"
	// Interactive screens use the alternate screen so shrinking frames leave no residue; the
	// summary is drawn inline so it stays in the scrollback after exit.
	v.AltScreen = m.state != stateDone
	return v
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.panel.SetWidth(width)
	m.list.resize(width-2*ui.HorizontalMargin, height-chromeLines)
}

// interrupt quits immediately, except while deleting: then it lets the current item finish.
func (m Model) interrupt() (tea.Model, tea.Cmd) {
	m.cancel()
	if m.state == stateCleaning {
		m.stopping = true
		m.panel.SetStatus(ui.Warning.Render("Stopping after the current item…"), "")
		return m, nil
	}
	return m.finish(true)
}

func (m Model) finish(aborted bool) (tea.Model, tea.Cmd) {
	m.state = stateDone
	m.aborted = aborted
	return m, tea.Quit
}
