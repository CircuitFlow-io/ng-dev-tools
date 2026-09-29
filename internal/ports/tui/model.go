// Package tui is the interactive terminal interface for `ngt ports`.
package tui

import (
	"context"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
	// chromeLines is the space taken by everything around the list on the selection screen.
	chromeLines = 12
)

type state int

const (
	stateLoading state = iota
	stateSelecting
	stateConfirming
	stateStopping
	stateDone
)

// Config wires the interface to the lister and stopper.
type Config struct {
	Lister  ports.Lister
	Stopper ports.Stopper
	// ShowSystem includes macOS's own daemons in the list.
	ShowSystem bool
}

// Model is the Bubble Tea model driving the ports flow.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config
	events chan tea.Msg

	state   state
	width   int
	height  int
	darkBG  bool
	spinner spinner.Model
	panel   ui.ProgressPanel
	list    processList
	hidden  int
	chosen  []ports.Process
	results []ports.StopResult
	err     error
	aborted bool
}

// New creates the model. Cancelling ctx stops any work in progress.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	return Model{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		events:  make(chan tea.Msg, ui.EventBuffer),
		width:   defaultWidth,
		height:  defaultHeight,
		darkBG:  true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
	}
}

// Err is the error that ended the flow, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts listing.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.startListing(), tea.RequestBackgroundColor)
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
	case stateLoading:
		return m.updateLoading(msg)
	case stateSelecting:
		return m.updateSelecting(msg)
	case stateConfirming:
		return m.updateConfirming(msg)
	case stateStopping:
		return m.updateStopping(msg)
	default:
		return m, nil
	}
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Looking for listening ports…")
	case stateSelecting:
		content = m.selectingView()
	case stateConfirming:
		content = m.confirmView()
	case stateStopping:
		content = m.panel.View()
	case stateDone:
		// The renderer clears the frame's last line on exit, so end the summary with a blank one.
		content = m.summaryView() + "\n"
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt ports"
	v.AltScreen = m.state != stateDone
	return v
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.panel.SetWidth(width)
	m.list.resize(width-2*ui.HorizontalMargin, height-chromeLines)
}

// interrupt quits at once, except while stopping: then it waits for the signals in flight.
func (m Model) interrupt() (tea.Model, tea.Cmd) {
	m.cancel()
	if m.state == stateStopping {
		return m, nil
	}
	return m.finish(true)
}

func (m Model) finish(aborted bool) (tea.Model, tea.Cmd) {
	m.state = stateDone
	m.aborted = aborted
	return m, tea.Quit
}
