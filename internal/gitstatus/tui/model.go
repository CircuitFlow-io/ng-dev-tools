// Package tui is the interactive terminal interface for `ngt status`.
package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide/idepicker"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth       = 110
	defaultHeight      = 30
	maxParallelFetches = 8
	// chromeLines is the space taken by everything around the table: the top margin, title, blank
	// line, column header, details box with its border, and help line with its margin.
	chromeLines = 6 + detailLines + 2
	listHelp    = "↑/↓ move · enter open in IDE · f fetch · F fetch all · r refresh · q quit"
	flashGap    = "   "
)

var errNoIDE = errors.New("no supported IDE found in /Applications or ~/Applications")

type state int

const (
	stateLoading state = iota
	stateListing
	stateChoosingIDE
	stateDone
)

// Config holds where the repositories are and how to act on them.
type Config struct {
	Root   string
	Home   string
	Runner macos.Runner
	// Fetch runs git fetch in a repository.
	Fetch func(ctx context.Context, dir string) error
	// FetchOnStart fetches every repository once the table is shown.
	FetchOnStart bool
	IDEs         []ide.IDE
	// ProjectIDEs maps a project path to the IDE app it was last opened in; DefaultIDE is the one
	// picked most recently for any project.
	ProjectIDEs map[string]string
	DefaultIDE  string
	// Open opens a project in an IDE and remembers the choice.
	Open func(path string, editor ide.IDE) error
}

type loadedMsg struct {
	repos []gitstatus.Repo
	err   error
}

type fetchedMsg struct {
	repo gitstatus.Repo
	err  error
}

type openedMsg struct {
	repo   gitstatus.Repo
	editor ide.IDE
	err    error
}

// Model is the Bubble Tea model driving the status screen.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	state      state
	width      int
	height     int
	darkBG     bool
	spinner    spinner.Model
	ticking    bool
	refreshing bool
	opening    bool
	table      table
	picker     idepicker.Picker
	pickingFor gitstatus.Repo
	flash      string
	fetchSlots chan struct{}
	err        error
}

// New creates the model. Cancelling ctx stops loading and fetching.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	if cfg.ProjectIDEs == nil {
		cfg.ProjectIDEs = map[string]string{}
	}
	m := Model{
		ctx:        ctx,
		cancel:     cancel,
		cfg:        cfg,
		width:      defaultWidth,
		height:     defaultHeight,
		darkBG:     true,
		spinner:    spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
		ticking:    true,
		table:      newTable(),
		fetchSlots: make(chan struct{}, maxParallelFetches),
	}
	m.resize(m.width, m.height)
	return m
}

// Err is the error that ended the screen, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts reading the repositories.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.loadAll(), tea.RequestBackgroundColor)
}

func (m Model) loadAll() tea.Cmd {
	return func() tea.Msg {
		repos, err := gitstatus.LoadAll(m.ctx, m.cfg.Runner, m.cfg.Root)
		return loadedMsg{repos: repos, err: err}
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
		m.table.highlight = ui.HighlightColor(m.darkBG)
		m.picker.SetHighlight(m.table.highlight)
		return m, nil
	case spinner.TickMsg:
		return m.tick(msg)
	case loadedMsg:
		return m.showRepos(msg)
	case fetchedMsg:
		return m.fetched(msg)
	case openedMsg:
		return m.opened(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
		return m.handleKey(msg)
	}
	return m, nil
}

func (m Model) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateListing:
		return m.updateListing(key)
	case stateChoosingIDE:
		return m.updateChoosingIDE(key)
	}
	return m, nil
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.table.resize(width-2*ui.HorizontalMargin, height-chromeLines)
}

// tick animates the spinner while anything is loading, and lets it stop otherwise.
func (m Model) tick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !m.busy() {
		m.ticking = false
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	m.table.spinner = m.spinner.View()
	return m, cmd
}

func (m Model) busy() bool {
	return m.state == stateLoading || m.refreshing || m.table.anyFetching()
}

func (m *Model) startTicking() tea.Cmd {
	m.table.spinner = m.spinner.View()
	if m.ticking {
		return nil
	}
	m.ticking = true
	return m.spinner.Tick
}

func (m Model) showRepos(msg loadedMsg) (tea.Model, tea.Cmd) {
	firstLoad := m.state == stateLoading
	m.refreshing = false
	if msg.err != nil {
		if firstLoad {
			m.err = msg.err
			return m.quit()
		}
		m.flash = ui.Warning.Render("Could not refresh: " + msg.err.Error())
		return m, nil
	}
	if firstLoad && len(msg.repos) == 0 {
		m.err = fmt.Errorf("no git repositories in %s", ui.TildePath(m.cfg.Root, m.cfg.Home))
		return m.quit()
	}
	m.table.setRepos(msg.repos)
	if firstLoad {
		m.state = stateListing
	}
	if firstLoad && m.cfg.FetchOnStart {
		return m.fetchAll()
	}
	return m, nil
}

func (m Model) updateListing(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch key.String() {
	case "q", "esc":
		return m.quit()
	case "enter":
		return m.chooseIDE()
	case "f":
		return m.fetchCurrent()
	case "F":
		return m.fetchAll()
	case "r":
		return m.refresh()
	}
	m.table.cursor.HandleKey(key.String())
	return m, nil
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	if m.refreshing {
		return m, nil
	}
	m.refreshing = true
	return m, tea.Batch(m.loadAll(), m.startTicking())
}

func (m Model) fetchCurrent() (tea.Model, tea.Cmd) {
	repo, ok := m.table.current()
	if !ok || m.table.fetching[repo.Path] {
		return m, nil
	}
	if !repo.HasRemote {
		m.flash = ui.Muted.Render(repo.Name + " has no remote to fetch from")
		return m, nil
	}
	return m, tea.Batch(m.fetch(repo), m.startTicking())
}

func (m Model) fetchAll() (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	for _, repo := range m.table.repos {
		if repo.HasRemote && !m.table.fetching[repo.Path] {
			cmds = append(cmds, m.fetch(repo))
		}
	}
	if len(cmds) == 0 {
		return m, nil
	}
	return m, tea.Batch(append(cmds, m.startTicking())...)
}

// fetch marks repo as fetching and returns the command that fetches it, at most
// maxParallelFetches at a time, and reads it again.
func (m Model) fetch(repo gitstatus.Repo) tea.Cmd {
	m.table.fetching[repo.Path] = true
	delete(m.table.fetchErrs, repo.Path)
	return func() tea.Msg {
		m.fetchSlots <- struct{}{}
		defer func() { <-m.fetchSlots }()
		err := m.cfg.Fetch(m.ctx, repo.Path)
		return fetchedMsg{repo: gitstatus.Load(m.ctx, m.cfg.Runner, repo.Name, repo.Path), err: err}
	}
}

func (m Model) fetched(msg fetchedMsg) (tea.Model, tea.Cmd) {
	delete(m.table.fetching, msg.repo.Path)
	if msg.err != nil {
		m.table.fetchErrs[msg.repo.Path] = msg.err.Error()
	}
	m.table.replace(msg.repo)
	return m, nil
}

// chooseIDE shows the IDE box for the repository under the cursor, or opens it straight away when
// only one IDE is installed.
func (m Model) chooseIDE() (tea.Model, tea.Cmd) {
	repo, ok := m.table.current()
	if !ok || m.opening {
		return m, nil
	}
	switch len(m.cfg.IDEs) {
	case 0:
		m.flash = ui.Warning.Render(errNoIDE.Error())
		return m, nil
	case 1:
		return m.open(repo, m.cfg.IDEs[0])
	}
	m.picker = idepicker.New(m.cfg.IDEs, m.cfg.ProjectIDEs[repo.Path], m.cfg.DefaultIDE)
	m.picker.SetHighlight(ui.HighlightColor(m.darkBG))
	m.pickingFor = repo
	m.state = stateChoosingIDE
	return m, nil
}

func (m Model) updateChoosingIDE(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.picker.HandleKey(key.String()) {
		return m, nil
	}
	switch key.String() {
	case "enter":
		m.state = stateListing
		return m.open(m.pickingFor, m.picker.Current())
	case "esc", "q":
		m.state = stateListing
	}
	return m, nil
}

func (m Model) open(repo gitstatus.Repo, editor ide.IDE) (tea.Model, tea.Cmd) {
	m.opening = true
	m.flash = ui.Muted.Render("Opening " + repo.Name + " in " + editor.Name + "…")
	return m, func() tea.Msg {
		return openedMsg{repo: repo, editor: editor, err: m.cfg.Open(repo.Path, editor)}
	}
}

func (m Model) opened(msg openedMsg) (tea.Model, tea.Cmd) {
	m.opening = false
	if msg.err != nil {
		m.flash = ui.Warning.Render(msg.err.Error())
		return m, nil
	}
	m.cfg.ProjectIDEs[msg.repo.Path] = msg.editor.AppPath
	m.cfg.DefaultIDE = msg.editor.AppPath
	m.flash = ui.Success.Render("Opened ") + ui.Bold.Render(msg.repo.Name) + ui.Success.Render(" in ") + ui.Bold.Render(msg.editor.Name)
	return m, nil
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Reading git status in "+ui.TildePath(m.cfg.Root, m.cfg.Home)+"…")
	case stateListing:
		content = m.listView()
	case stateChoosingIDE:
		content = m.picker.View(m.pickingFor.Name)
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt status"
	v.AltScreen = true
	return v
}

func (m Model) listView() string {
	width := m.width - 2*ui.HorizontalMargin
	repo, ok := m.table.current()
	lines := []string{
		m.title(),
		"",
		m.table.header(),
		m.table.view(),
		details(repo, ok, m.table.syncState(repo), m.table.now, width),
		m.help(),
	}
	return strings.Join(lines, "\n")
}

func (m Model) title() string {
	facts := ui.Count(len(m.table.repos), "repo") + " in " + ui.TildePath(m.cfg.Root, m.cfg.Home)
	summary := ui.Success.Render("all clean")
	if n := m.table.needingAttention(); n > 0 {
		summary = ui.Warning.Render(fmt.Sprintf("%d need attention", n))
	}
	title := ui.Title.Render("Status") + ui.Muted.Render("  "+facts+noteJoiner) + summary
	switch {
	case m.refreshing:
		title += ui.Muted.Render(noteJoiner + "refreshing…")
	case m.table.anyFetching():
		title += ui.Muted.Render(noteJoiner + fmt.Sprintf("fetching %d…", len(m.table.fetching)))
	}
	return title
}

func (m Model) help() string {
	if m.flash == "" {
		return ui.Help.Render(listHelp)
	}
	return ui.Help.Render(m.flash + flashGap + ui.Muted.Render(listHelp))
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}
