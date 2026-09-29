// Package tui is the interactive terminal interface for `ngt run`.
package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
	"github.com/nasserghiasi/ng-dev-tools/internal/projects"
	"github.com/nasserghiasi/ng-dev-tools/internal/projects/projectlist"
	"github.com/nasserghiasi/ng-dev-tools/internal/scripts"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
	// projectChromeLines is the space taken by everything around the list on the project screen.
	projectChromeLines = 9
	// scriptChromeLines is the space taken by everything around the lists on the script screen:
	// margin, title, search, blank line, header, the details box with its border, and help.
	scriptChromeLines = 5 + detailLines + 2 + 2
	usedVerb          = "used"
	projectHelp       = "type to filter · ↑/↓ move · enter show scripts · esc clear or quit"
	paneHelp          = "type to search · ↑/↓ script · ←/→ package · enter run · esc "
	singleHelp        = "type to search · ↑/↓ move · enter run · esc "
	searchHelp        = "↑/↓ move · enter run · esc clear search"
)

type state int

const (
	stateScanning state = iota
	stateChoosingProject
	stateLoading
	stateChoosingScript
	stateDone
)

// Config holds what the interface lists and remembers.
type Config struct {
	// Root is the projects folder, listed when Workspace is nil.
	Root string
	Home string
	// Workspace is the project ngt was started in, if any.
	Workspace *scripts.Workspace
	// CurrentDir focuses the package it lies in.
	CurrentDir string
	// Query starts the search on the script screen, or the filter on the project screen.
	Query   string
	History scripts.History
	// Used maps project paths to when ngt last opened them or ran a script there.
	Used   map[string]time.Time
	NVMDir string
	Runner macos.Runner
}

type scannedMsg struct {
	projects []projects.Project
	err      error
}

type loadedMsg struct {
	workspace scripts.Workspace
	err       error
}

// Model is the Bubble Tea model driving the run flow.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	state       state
	width       int
	height      int
	darkBG      bool
	spinner     spinner.Model
	list        projectlist.List
	fromList    bool
	picker      scriptPicker
	chosen      scripts.Target
	hasChoice   bool
	loadingName string
	err         error
}

// New creates the model. Cancelling ctx stops the project scan.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	m := Model{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		width:   defaultWidth,
		height:  defaultHeight,
		darkBG:  true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
	}
	if cfg.Workspace != nil {
		m.showScripts(*cfg.Workspace, cfg.CurrentDir, cfg.Query)
	}
	return m
}

// Chosen is the workspace and script picked, or ok false when the user quit.
func (m Model) Chosen() (scripts.Workspace, scripts.Target, bool) {
	return m.picker.ws, m.chosen, m.hasChoice
}

// Err is the error that ended the flow, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts scanning for projects unless ngt was started inside one.
func (m Model) Init() tea.Cmd {
	if m.state == stateChoosingScript {
		return tea.RequestBackgroundColor
	}
	return tea.Batch(m.spinner.Tick, m.scan(), tea.RequestBackgroundColor)
}

func (m Model) scan() tea.Cmd {
	return func() tea.Msg {
		found, err := projects.Scan(m.ctx, m.cfg.Root, m.cfg.Used)
		if err != nil {
			return scannedMsg{err: err}
		}
		var runnable []projects.Project
		for _, p := range found {
			if scripts.IsProject(p.Path) {
				runnable = append(runnable, p)
			}
		}
		return scannedMsg{projects: runnable}
	}
}

func load(root string) tea.Cmd {
	return func() tea.Msg {
		ws, err := scripts.Load(root)
		return loadedMsg{workspace: ws, err: err}
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
		m.picker.highlight = ui.HighlightColor(m.darkBG)
		return m, nil
	case projectlist.DirtyMsg:
		m.list.SetDirty(msg.Path, msg.Dirty)
		return m, nil
	case spinner.TickMsg:
		if m.state != stateScanning && m.state != stateLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
	}

	switch m.state {
	case stateScanning:
		if msg, ok := msg.(scannedMsg); ok {
			return m.showProjects(msg)
		}
	case stateChoosingProject:
		return m.updateChoosingProject(msg)
	case stateLoading:
		if msg, ok := msg.(loadedMsg); ok {
			return m.loaded(msg)
		}
	case stateChoosingScript:
		return m.updateChoosingScript(msg)
	}
	return m, nil
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	inner := width - 2*ui.HorizontalMargin
	m.list.Resize(inner, height-projectChromeLines)
	m.picker.resize(inner, height-scriptChromeLines)
}

func (m Model) showProjects(msg scannedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m.quit()
	}
	if len(msg.projects) == 0 {
		m.err = fmt.Errorf("no npm or pnpm projects in %s", ui.TildePath(m.cfg.Root, m.cfg.Home))
		return m.quit()
	}
	m.list = projectlist.New(msg.projects, m.cfg.Home, time.Now(), usedVerb)
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

func (m Model) chooseProject() (tea.Model, tea.Cmd) {
	project, ok := m.list.Current()
	if !ok {
		return m, nil
	}
	m.state = stateLoading
	m.loadingName = project.Name
	return m, tea.Batch(m.spinner.Tick, load(project.Path))
}

func (m Model) loaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m.quit()
	}
	m.fromList = true
	m.showScripts(msg.workspace, "", "")
	return m, nil
}

func (m *Model) showScripts(ws scripts.Workspace, currentDir, query string) {
	m.picker = newScriptPicker(ws, m.cfg.History.Recent(ws.Root), currentDir, m.cfg.NVMDir)
	m.picker.highlight = ui.HighlightColor(m.darkBG)
	m.picker.setQuery(query)
	m.resize(m.width, m.height)
	m.state = stateChoosingScript
}

func (m Model) updateChoosingScript(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "enter":
		return m.choose()
	case "esc":
		return m.back()
	}
	m.picker.handleKey(key)
	return m, nil
}

// back clears the search, then returns to the project list, or quits when there is none.
func (m Model) back() (tea.Model, tea.Cmd) {
	if m.picker.searching() {
		m.picker.setQuery("")
		return m, nil
	}
	if !m.fromList {
		return m.quit()
	}
	m.state = stateChoosingProject
	return m, nil
}

func (m Model) choose() (tea.Model, tea.Cmd) {
	target, ok := m.picker.current()
	if !ok {
		return m, nil
	}
	m.chosen, m.hasChoice = target, true
	m.state = stateDone
	return m, tea.Quit
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateScanning:
		content = m.spinner.View() + " " + ui.Title.Render("Looking through "+ui.TildePath(m.cfg.Root, m.cfg.Home)+"…")
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Reading the scripts of "+m.loadingName+"…")
	case stateChoosingProject:
		content = m.projectView()
	case stateChoosingScript:
		content = m.scriptView()
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt run"
	v.AltScreen = true
	return v
}

func (m Model) projectView() string {
	title := ui.Title.Render("Run a script") +
		ui.Muted.Render(fmt.Sprintf("  %s in %s", ui.Count(m.list.Len(), "project"), ui.TildePath(m.cfg.Root, m.cfg.Home)))
	return strings.Join([]string{
		title,
		m.list.FilterLine(),
		"",
		m.list.Header(),
		m.list.View(),
		ui.Help.Render(projectHelp),
	}, "\n")
}

func (m Model) scriptView() string {
	return strings.Join([]string{
		m.scriptTitle(),
		projectlist.FilterLine(m.picker.query, "type to search all scripts"),
		"",
		m.picker.header(),
		m.picker.rows(),
		m.picker.details(),
		ui.Help.Render(m.scriptHelp()),
	}, "\n")
}

func (m Model) scriptTitle() string {
	ws := m.picker.ws
	facts := []string{string(ws.Manager)}
	if hasPackagePane(ws) {
		facts = append(facts, ui.Count(len(ws.Packages), "package"))
	}
	facts = append(facts, ui.Count(ws.ScriptCount(), "script"))
	return ui.Title.Render("Run in "+ws.Name()) + ui.Muted.Render("  "+strings.Join(facts, " · "))
}

func (m Model) scriptHelp() string {
	if m.picker.searching() {
		return searchHelp
	}
	leave := "quit"
	if m.fromList {
		leave = "back"
	}
	if hasPackagePane(m.picker.ws) {
		return paneHelp + leave
	}
	return singleHelp + leave
}
