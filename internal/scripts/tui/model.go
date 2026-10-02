// Package tui is the interactive terminal interface for `ngt run`.
package tui

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/projectlist"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/scripts"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 100
	defaultHeight = 30
	// chromeLines is the space taken by everything around the columns: margin, title, search,
	// blank line, header, the details box with its border, and help.
	chromeLines   = 5 + detailLines + 2 + 2
	projectHelp   = "type to filter · ↑/↓ project · → or enter next · esc "
	packageHelp   = "type to search · ↑/↓ package · ←/→ column · enter scripts · esc quit"
	scriptHelp    = "type to search · ↑/↓ script · ← back · enter run · esc quit"
	searchHelp    = "↑/↓ move · ← projects · enter run · esc clear search"
	loadingFormat = "Reading the scripts of %s…"
)

// focus is the column the arrow keys move in.
type focus int

const (
	focusProjects focus = iota
	focusPackages
	focusScripts
)

// Config holds what the interface lists and remembers.
type Config struct {
	// Root is the projects folder listed in the projects column.
	Root string
	Home string
	// Workspace is the project ngt was started in, if any. It is selected, and listed even when
	// it is not in Root.
	Workspace *scripts.Workspace
	// CurrentDir selects the package it lies in.
	CurrentDir string
	// Query starts the script search inside Workspace, or the project filter without one.
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
	root      string
	workspace scripts.Workspace
	err       error
}

// loadResult is a project's scripts, read once and kept while the screen is open.
type loadResult struct {
	workspace scripts.Workspace
	err       error
}

// Model is the Bubble Tea model driving the run screen.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	width     int
	height    int
	highlight color.Color
	spinner   spinner.Model
	scanning  bool
	projects  projectColumn
	loaded    map[string]loadResult
	picker    *scriptPicker
	focus     focus
	chosen    scripts.Target
	hasChoice bool
	done      bool
	err       error
}

// New creates the model. Cancelling ctx stops the project scan.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	m := Model{
		ctx:       ctx,
		cancel:    cancel,
		cfg:       cfg,
		width:     defaultWidth,
		height:    defaultHeight,
		highlight: ui.HighlightColor(true),
		spinner:   spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
		scanning:  true,
		loaded:    map[string]loadResult{},
	}
	if ws := cfg.Workspace; ws != nil {
		m.loaded[ws.Root] = loadResult{workspace: *ws}
		m.projects = newProjectColumn([]projects.Project{workspaceProject(*ws)}, ws.Root)
		m.focus = focusScripts
		m.showPicker()
		m.picker.setQuery(cfg.Query)
	}
	m.resize(m.width, m.height)
	return m
}

func workspaceProject(ws scripts.Workspace) projects.Project {
	return projects.Project{Name: ws.Name(), Path: ws.Root}
}

// Chosen is the workspace and script picked, or ok false when the user quit.
func (m Model) Chosen() (scripts.Workspace, scripts.Target, bool) {
	if m.picker == nil {
		return scripts.Workspace{}, scripts.Target{}, false
	}
	return m.picker.ws, m.chosen, m.hasChoice
}

// Err is the error that ended the flow, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts scanning for projects.
func (m Model) Init() tea.Cmd {
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
		return loadedMsg{root: root, workspace: ws, err: err}
	}
}

// Update routes messages to their handlers.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
	case tea.BackgroundColorMsg:
		m.highlight = ui.HighlightColor(msg.IsDark())
		if m.picker != nil {
			m.picker.highlight = m.highlight
		}
	case spinner.TickMsg:
		if !m.busy() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case scannedMsg:
		return m.showProjects(msg)
	case loadedMsg:
		return m.workspaceLoaded(msg)
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

// busy reports whether a spinner is on screen.
func (m Model) busy() bool {
	return m.scanning || m.loadingCurrent()
}

func (m Model) loadingCurrent() bool {
	project, ok := m.projects.current()
	if !ok {
		return false
	}
	_, done := m.loaded[project.Path]
	return !done
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	rows := height - chromeLines
	m.projects.resize(rows)
	if m.picker != nil {
		m.picker.resize(m.innerWidth()-m.projects.width(), rows)
	}
}

func (m Model) innerWidth() int {
	return m.width - 2*ui.HorizontalMargin
}

// showProjects fills the projects column once the scan is done, keeping the project selected so
// far, or else selecting the one a script last ran in.
func (m Model) showProjects(msg scannedMsg) (tea.Model, tea.Cmd) {
	m.scanning = false
	if msg.err != nil && m.cfg.Workspace == nil {
		m.err = msg.err
		return m.quit()
	}
	found := msg.projects
	if ws := m.cfg.Workspace; ws != nil && !containsProject(found, ws.Root) {
		found = append([]projects.Project{workspaceProject(*ws)}, found...)
	}
	if len(found) == 0 {
		m.err = fmt.Errorf("no npm or pnpm projects in %s", ui.TildePath(m.cfg.Root, m.cfg.Home))
		return m.quit()
	}

	m.projects = newProjectColumn(found, m.initialProject())
	m.resize(m.width, m.height)
	if m.cfg.Workspace != nil {
		return m, nil
	}
	if m.cfg.Query != "" {
		m.projects.setQuery(m.cfg.Query)
		m.focus = focusProjects
		if len(m.projects.shown) == 1 {
			m.focus = focusScripts
		}
	} else if _, _, ranBefore := m.cfg.History.LastAnywhere(); ranBefore {
		m.focus = focusScripts
	}
	return m.selectProject()
}

func (m Model) initialProject() string {
	if m.cfg.Workspace != nil {
		return m.cfg.Workspace.Root
	}
	root, _, _ := m.cfg.History.LastAnywhere()
	return root
}

func containsProject(found []projects.Project, path string) bool {
	for _, p := range found {
		if p.Path == path {
			return true
		}
	}
	return false
}

// selectProject shows the scripts of the project under the cursor, reading them first when they
// have not been read yet.
func (m Model) selectProject() (tea.Model, tea.Cmd) {
	m.picker = nil
	project, ok := m.projects.current()
	if !ok {
		return m, nil
	}
	if _, done := m.loaded[project.Path]; !done {
		return m, tea.Batch(m.spinner.Tick, load(project.Path))
	}
	m.showPicker()
	return m, nil
}

func (m Model) workspaceLoaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	m.loaded[msg.root] = loadResult{workspace: msg.workspace, err: msg.err}
	if project, ok := m.projects.current(); ok && project.Path == msg.root {
		m.showPicker()
	}
	return m, nil
}

// showPicker builds the packages and scripts columns of the selected project, on the package and
// script that ran there last.
func (m *Model) showPicker() {
	m.picker = nil
	project, ok := m.projects.current()
	if !ok {
		return
	}
	result := m.loaded[project.Path]
	if result.err != nil || result.workspace.ScriptCount() == 0 {
		return
	}
	ws := result.workspace
	currentDir := ""
	if m.cfg.Workspace != nil && m.cfg.Workspace.Root == ws.Root {
		currentDir = m.cfg.CurrentDir
	}
	picker := newScriptPicker(ws, m.cfg.History.Recent(ws.Root), currentDir, m.cfg.NVMDir)
	picker.highlight = m.highlight
	m.picker = &picker
	if m.focus == focusPackages && !picker.hasPackageColumn() {
		m.focus = focusScripts
	}
	m.resize(m.width, m.height)
}

func (m Model) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "ctrl+c":
		return m.quit()
	case "enter":
		return m.enter()
	case "esc":
		return m.escape()
	case "right", "tab":
		m.moveFocus(1)
		return m, nil
	case "left", "shift+tab":
		m.moveFocus(-1)
		return m, nil
	}
	if m.scanning && m.cfg.Workspace == nil {
		return m, nil
	}
	if m.focus == focusProjects {
		return m.handleProjectKey(key)
	}
	if m.picker != nil {
		m.picker.handleKey(key, m.focus)
		if m.picker.searching() {
			m.focus = focusScripts
		}
	}
	return m, nil
}

func (m Model) handleProjectKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	before, _ := m.projects.current()
	if !m.projects.handleKey(key) {
		return m, nil
	}
	if after, _ := m.projects.current(); after.Path == before.Path && m.picker != nil {
		return m, nil
	}
	return m.selectProject()
}

// columns lists the columns shown, left to right.
func (m Model) columns() []focus {
	if m.picker != nil && m.picker.hasPackageColumn() {
		return []focus{focusProjects, focusPackages, focusScripts}
	}
	return []focus{focusProjects, focusScripts}
}

// moveFocus moves to the next or previous column, stopping at either end.
func (m *Model) moveFocus(delta int) {
	columns := m.columns()
	i := 0
	for j, f := range columns {
		if f == m.focus {
			i = j
		}
	}
	m.focus = columns[max(0, min(len(columns)-1, i+delta))]
}

// enter moves on to the next column, or runs the script under the cursor in the last one.
func (m Model) enter() (tea.Model, tea.Cmd) {
	if m.focus != focusScripts {
		m.moveFocus(1)
		return m, nil
	}
	if m.picker == nil {
		return m, nil
	}
	target, ok := m.picker.current()
	if !ok {
		return m, nil
	}
	m.chosen, m.hasChoice = target, true
	m.done = true
	return m, tea.Quit
}

// escape clears the search or the project filter, then quits.
func (m Model) escape() (tea.Model, tea.Cmd) {
	if m.focus != focusProjects && m.picker != nil && m.picker.searching() {
		m.picker.setQuery("")
		return m, nil
	}
	if m.projects.query != "" {
		m.projects.setQuery("")
		return m.selectProject()
	}
	return m.quit()
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.done = true
	return m, tea.Quit
}

// View renders the screen.
func (m Model) View() tea.View {
	if m.done {
		return tea.NewView("")
	}
	content := m.scanningView()
	if !m.scanning || m.cfg.Workspace != nil {
		content = m.columnsView()
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt run"
	v.AltScreen = true
	return v
}

func (m Model) scanningView() string {
	return m.spinner.View() + " " + ui.Title.Render("Looking through "+ui.TildePath(m.cfg.Root, m.cfg.Home)+"…")
}

func (m Model) columnsView() string {
	return strings.Join([]string{
		m.title(),
		m.filterLine(),
		"",
		m.header(),
		m.rows(),
		m.details(),
		ui.Help.Render(m.help()),
	}, "\n")
}

func (m Model) title() string {
	if m.picker == nil {
		return ui.Title.Render("Run a script") + ui.Muted.Render("  "+ui.Count(len(m.projects.all), "project"))
	}
	ws := m.picker.ws
	facts := []string{string(ws.Manager)}
	if hasPackagePane(ws) {
		facts = append(facts, ui.Count(len(ws.Packages), "package"))
	}
	facts = append(facts, ui.Count(ws.ScriptCount(), "script"))
	return ui.Title.Render("Run in "+ws.Name()) + ui.Muted.Render("  "+strings.Join(facts, " · "))
}

func (m Model) filterLine() string {
	if m.focus == focusProjects || m.picker == nil {
		return projectlist.FilterLine(m.projects.query, "type to filter projects")
	}
	return projectlist.FilterLine(m.picker.query, "type to search all scripts")
}

func (m Model) header() string {
	left := ui.Muted.Render(ui.PadRight("  PROJECTS", m.projects.width()))
	if m.picker == nil {
		return left + ui.Muted.Render("  SCRIPTS")
	}
	return left + m.picker.header()
}

// rows renders the columns side by side, always as tall as the space they have so the details
// box below stays in place.
func (m Model) rows() string {
	left := m.projects.rows(m.focus == focusProjects, m.highlight)
	right := m.rightRows()
	height := max(1, m.height-chromeLines)
	lines := make([]string, height)
	for i := range lines {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines[i] = ui.PadRight(l, m.projects.width()) + r
	}
	return strings.Join(lines, "\n")
}

func (m Model) rightRows() []string {
	if m.picker != nil {
		return m.picker.rows(m.focus)
	}
	project, ok := m.projects.current()
	if !ok {
		return nil
	}
	result, done := m.loaded[project.Path]
	switch {
	case !done:
		return []string{"  " + m.spinner.View() + " " + ui.Muted.Render(fmt.Sprintf(loadingFormat, project.Name))}
	case result.err != nil:
		return []string{"  " + ui.Warning.Render(ui.Truncate("Could not read the scripts: "+result.err.Error(), m.innerWidth()-m.projects.width()-cursorWidth))}
	default:
		return []string{ui.Muted.Render("  No scripts")}
	}
}

func (m Model) details() string {
	if m.picker == nil {
		return messageBox(m.innerWidth(), ui.Muted.Render("No script selected"))
	}
	return m.picker.details(m.innerWidth())
}

func (m Model) help() string {
	switch {
	case m.focus == focusProjects:
		if m.projects.query != "" {
			return projectHelp + "clear"
		}
		return projectHelp + "quit"
	case m.picker != nil && m.picker.searching():
		return searchHelp
	case m.focus == focusPackages:
		return packageHelp
	default:
		return scriptHelp
	}
}
