// Package tui is the interactive terminal interface for `ngt open`.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide/idepicker"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/projectlist"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
	// chromeLines is the space taken by everything around the list on the project screen.
	chromeLines = 9
	projectHelp = "type to filter · ↑/↓ move · enter choose IDE · esc clear or quit"
)

type state int

const (
	stateScanning state = iota
	stateChoosingProject
	stateChoosingIDE
	stateDone
)

// Config holds what the interface lists and remembers.
type Config struct {
	Root   string
	Home   string
	Query  string
	State  projects.State
	IDEs   []ide.IDE
	Runner macos.Runner
}

type scannedMsg struct {
	projects []projects.Project
	err      error
}

// Model is the Bubble Tea model driving the open flow.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	state   state
	width   int
	height  int
	darkBG  bool
	spinner spinner.Model
	list    projectlist.List
	picker  idepicker.Picker
	project projects.Project
	editor  ide.IDE
	chosen  bool
	err     error
}

// New creates the model. Cancelling ctx stops the scan.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	return Model{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		width:   defaultWidth,
		height:  defaultHeight,
		darkBG:  true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
	}
}

// Chosen is the project and IDE picked, or ok false when the user quit.
func (m Model) Chosen() (project projects.Project, editor ide.IDE, ok bool) {
	return m.project, m.editor, m.chosen
}

// Err is the error that ended the flow, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts scanning.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.scan(), tea.RequestBackgroundColor)
}

func (m Model) scan() tea.Cmd {
	return func() tea.Msg {
		found, err := projects.Scan(m.ctx, m.cfg.Root, m.cfg.State.Opened)
		return scannedMsg{projects: found, err: err}
	}
}

// Update routes messages to the handler for the current screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.BackgroundColorMsg:
		m.darkBG = msg.IsDark()
		m.list.SetHighlight(ui.HighlightColor(m.darkBG))
		m.picker.SetHighlight(ui.HighlightColor(m.darkBG))
		return m, nil
	case projectlist.DirtyMsg:
		m.list.SetDirty(msg.Path, msg.Dirty)
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
	}

	switch m.state {
	case stateScanning:
		return m.updateScanning(msg)
	case stateChoosingProject:
		return m.updateChoosingProject(msg)
	case stateChoosingIDE:
		return m.updateChoosingIDE(msg)
	default:
		return m, nil
	}
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateScanning:
		content = m.spinner.View() + " " + ui.Title.Render("Looking through "+ui.TildePath(m.cfg.Root, m.cfg.Home)+"…")
	case stateChoosingProject:
		content = m.projectView()
	case stateChoosingIDE:
		content = m.picker.View(m.project.Name)
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt open"
	v.AltScreen = true
	return v
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.list.Resize(width-2*ui.HorizontalMargin, height-chromeLines)
}

func (m Model) updateScanning(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case scannedMsg:
		return m.showProjects(msg)
	}
	return m, nil
}

func (m Model) showProjects(msg scannedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m.quit()
	}
	if len(msg.projects) == 0 {
		m.err = fmt.Errorf("no projects in %s", ui.TildePath(m.cfg.Root, m.cfg.Home))
		return m.quit()
	}

	m.list = projectlist.New(msg.projects, m.cfg.Home, time.Now(), "opened")
	m.list.SetHighlight(ui.HighlightColor(m.darkBG))
	m.resize(m.width, m.height)
	m.list.SetQuery(m.cfg.Query)
	m.state = stateChoosingProject

	checks := projectlist.DirtyChecks(m.ctx, m.cfg.Runner, msg.projects)
	if m.cfg.Query != "" && len(m.list.Shown()) == 1 {
		next, cmd := m.chooseProject()
		return next, tea.Batch(checks, cmd)
	}
	return m, checks
}

func (m Model) updateChoosingProject(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "enter":
		return m.chooseProject()
	case "esc":
		if m.list.Query() == "" {
			return m.quit()
		}
		m.list.SetQuery("")
		return m, nil
	}
	m.list.HandleKey(key)
	return m, nil
}

// chooseProject moves to the IDE box, or finishes straight away when only one IDE is installed.
func (m Model) chooseProject() (tea.Model, tea.Cmd) {
	project, ok := m.list.Current()
	if !ok {
		return m, nil
	}
	m.project = project
	if len(m.cfg.IDEs) == 1 {
		return m.finish(m.cfg.IDEs[0])
	}
	m.picker = idepicker.New(m.cfg.IDEs, m.cfg.State.ProjectIDEs[project.Path], m.cfg.State.IDE)
	m.picker.SetHighlight(ui.HighlightColor(m.darkBG))
	m.state = stateChoosingIDE
	return m, nil
}

func (m Model) updateChoosingIDE(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.picker.HandleKey(key.String()) {
		return m, nil
	}
	switch key.String() {
	case "enter":
		return m.finish(m.picker.Current())
	case "esc", "q":
		m.state = stateChoosingProject
	}
	return m, nil
}

func (m Model) projectView() string {
	title := ui.Title.Render("Open a project") +
		ui.Muted.Render(fmt.Sprintf("  %s in %s", ui.Count(m.list.Len(), "project"), ui.TildePath(m.cfg.Root, m.cfg.Home)))
	lines := []string{
		title,
		m.list.FilterLine(),
		"",
		m.list.Header(),
		m.list.View(),
		ui.Help.Render(projectHelp),
	}
	return strings.Join(lines, "\n")
}

func (m Model) finish(editor ide.IDE) (tea.Model, tea.Cmd) {
	m.editor = editor
	m.chosen = true
	m.state = stateDone
	return m, tea.Quit
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}
