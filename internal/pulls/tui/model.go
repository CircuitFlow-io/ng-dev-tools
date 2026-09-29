// Package tui is the interactive terminal interface for `ngt prs`.
package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide/idepicker"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 110
	defaultHeight = 30
	// chromeLines is the space taken by everything around the list: the top margin, title, blank
	// line, details box with its border, and help line with its margin.
	chromeLines = 5 + detailLines + 2
	listHelp    = "↑/↓ move · enter browser · c check out · i IDE · l log · r refresh · q quit"
	flashGap    = "   "
)

var errNoIDE = errors.New("no supported IDE found in /Applications or ~/Applications")

type state int

const (
	stateLoading state = iota
	stateListing
	stateChoosingIDE
	stateViewingLog
	stateDone
)

// Config holds how the screen reads pull requests and acts on them.
type Config struct {
	Root string
	Home string
	// Load reads the pull requests involving you.
	Load func(ctx context.Context) (pulls.Dashboard, error)
	// Clones maps "owner/name" to the local project cloned from it.
	Clones func() map[string]string
	// Checkout switches a local clone to a pull request's branch.
	Checkout func(ctx context.Context, dir string, p pulls.PR) error
	// FailedLog reads the log of a failed check.
	FailedLog func(ctx context.Context, check pulls.Check) ([]pulls.LogLine, error)
	// OpenURL opens a page in the browser.
	OpenURL func(url string) error
	IDEs    []ide.IDE
	// ProjectIDEs maps a project path to the IDE app it was last opened in; DefaultIDE is the one
	// picked most recently for any project.
	ProjectIDEs map[string]string
	DefaultIDE  string
	// Open opens a project in an IDE and remembers the choice.
	Open func(path string, editor ide.IDE) error
}

type loadedMsg struct {
	dashboard pulls.Dashboard
	clones    map[string]string
	err       error
}

// actionMsg ends an action on a pull request with a line to show, or the error it met.
type actionMsg struct {
	done string
	err  error
	// opened is set when a project was opened in an IDE, to remember the choice.
	opened *openedIn
}

type openedIn struct {
	path   string
	editor ide.IDE
}

type logMsg struct {
	check pulls.Check
	lines []pulls.LogLine
	err   error
}

// Model is the Bubble Tea model driving the pull requests screen.
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
	working    bool
	list       list
	clones     map[string]string
	picker     idepicker.Picker
	pickingFor string
	logs       logView
	flash      string
	err        error
}

// New creates the model. Cancelling ctx stops what is loading.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	if cfg.ProjectIDEs == nil {
		cfg.ProjectIDEs = map[string]string{}
	}
	m := Model{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		width:   defaultWidth,
		height:  defaultHeight,
		darkBG:  true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
		ticking: true,
		list:    newList(pulls.Dashboard{}),
	}
	m.resize(m.width, m.height)
	return m
}

// Err is the error that ended the screen, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts loading.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load(), tea.RequestBackgroundColor)
}

func (m Model) load() tea.Cmd {
	return func() tea.Msg {
		d, err := m.cfg.Load(m.ctx)
		return loadedMsg{dashboard: d, clones: m.cfg.Clones(), err: err}
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
		m.list.highlight = ui.HighlightColor(m.darkBG)
		m.picker.SetHighlight(m.list.highlight)
		return m, nil
	case spinner.TickMsg:
		return m.tick(msg)
	case loadedMsg:
		return m.loaded(msg)
	case actionMsg:
		return m.actionDone(msg)
	case logMsg:
		return m.logLoaded(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
		return m.handleKey(msg)
	}
	if m.state == stateViewingLog {
		var cmd tea.Cmd
		m.logs, cmd = m.logs.update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch m.state {
	case stateListing:
		return m.updateListing(key)
	case stateChoosingIDE:
		return m.updateChoosingIDE(key)
	case stateViewingLog:
		return m.updateViewingLog(key)
	}
	return m, nil
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.list.resize(width-2*ui.HorizontalMargin, height-chromeLines)
	m.logs.resize(width-2*ui.HorizontalMargin, height)
}

func (m Model) busy() bool {
	return m.state == stateLoading || m.refreshing || m.working || (m.state == stateViewingLog && m.logs.loading)
}

// tick animates the spinner while anything is loading, and lets it stop otherwise.
func (m Model) tick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !m.busy() {
		m.ticking = false
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m *Model) startTicking() tea.Cmd {
	if m.ticking {
		return nil
	}
	m.ticking = true
	return m.spinner.Tick
}

func (m Model) loaded(msg loadedMsg) (tea.Model, tea.Cmd) {
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
	current, hadCurrent := m.list.current()
	highlight := m.list.highlight
	m.list = newList(msg.dashboard)
	m.list.highlight = highlight
	m.clones = msg.clones
	m.resize(m.width, m.height)
	if hadCurrent {
		m.list.keepCurrent(current.URL)
	}
	if firstLoad {
		m.state = stateListing
	}
	return m, nil
}

func (m Model) updateListing(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch key.String() {
	case "q", "esc":
		return m.quit()
	case "r":
		return m.refresh()
	}
	if m.list.handleKey(key.String()) {
		return m, nil
	}
	p, ok := m.list.current()
	if !ok || m.working {
		return m, nil
	}
	switch key.String() {
	case "enter":
		return m.openInBrowser(p.URL, p.Ref())
	case "c":
		return m.checkout(p)
	case "i":
		return m.chooseIDE(p)
	case "l":
		return m.showLog(p)
	}
	return m, nil
}

func (m Model) refresh() (tea.Model, tea.Cmd) {
	if m.refreshing {
		return m, nil
	}
	m.refreshing = true
	return m, tea.Batch(m.load(), m.startTicking())
}

func (m Model) openInBrowser(url, name string) (tea.Model, tea.Cmd) {
	return m, func() tea.Msg {
		if err := m.cfg.OpenURL(url); err != nil {
			return actionMsg{err: fmt.Errorf("could not open %s: %w", name, err)}
		}
		return actionMsg{done: "Opened " + name + " in your browser"}
	}
}

// clone is the local project holding p's repository, if it is cloned.
func (m Model) clone(p pulls.PR) (string, bool) {
	return pulls.LocalClone(m.clones, p.Repo)
}

func (m Model) notCloned(p pulls.PR) (tea.Model, tea.Cmd) {
	m.flash = ui.Warning.Render(p.Repo + " is not cloned in " + ui.TildePath(m.cfg.Root, m.cfg.Home))
	return m, nil
}

func (m Model) checkout(p pulls.PR) (tea.Model, tea.Cmd) {
	dir, ok := m.clone(p)
	if !ok {
		return m.notCloned(p)
	}
	m.working = true
	name := filepath.Base(dir)
	m.flash = ui.Muted.Render("Checking out " + p.HeadRef + " in " + name + "…")
	previous := projects.Branch(dir)
	return m, tea.Batch(m.startTicking(), func() tea.Msg {
		if err := m.cfg.Checkout(m.ctx, dir, p); err != nil {
			return actionMsg{err: fmt.Errorf("could not check out %s in %s: %w", p.Ref(), name, err)}
		}
		done := "Checked out " + p.HeadRef + " in " + name
		if previous != "" && previous != p.HeadRef {
			done += " (was " + previous + ")"
		}
		return actionMsg{done: done}
	})
}

// chooseIDE shows the IDE box for p's local project, or opens it straight away when only one IDE
// is installed.
func (m Model) chooseIDE(p pulls.PR) (tea.Model, tea.Cmd) {
	dir, ok := m.clone(p)
	if !ok {
		return m.notCloned(p)
	}
	switch len(m.cfg.IDEs) {
	case 0:
		m.flash = ui.Warning.Render(errNoIDE.Error())
		return m, nil
	case 1:
		return m.openInIDE(dir, m.cfg.IDEs[0])
	}
	m.picker = idepicker.New(m.cfg.IDEs, m.cfg.ProjectIDEs[dir], m.cfg.DefaultIDE)
	m.picker.SetHighlight(ui.HighlightColor(m.darkBG))
	m.pickingFor = dir
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
		return m.openInIDE(m.pickingFor, m.picker.Current())
	case "esc", "q":
		m.state = stateListing
	}
	return m, nil
}

func (m Model) openInIDE(dir string, editor ide.IDE) (tea.Model, tea.Cmd) {
	m.working = true
	name := filepath.Base(dir)
	m.flash = ui.Muted.Render("Opening " + name + " in " + editor.Name + "…")
	return m, tea.Batch(m.startTicking(), func() tea.Msg {
		if err := m.cfg.Open(dir, editor); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{done: "Opened " + name + " in " + editor.Name, opened: &openedIn{path: dir, editor: editor}}
	})
}

func (m Model) actionDone(msg actionMsg) (tea.Model, tea.Cmd) {
	m.working = false
	if msg.err != nil {
		m.flash = ui.Warning.Render(msg.err.Error())
		return m, nil
	}
	if msg.opened != nil {
		m.cfg.ProjectIDEs[msg.opened.path] = msg.opened.editor.AppPath
		m.cfg.DefaultIDE = msg.opened.editor.AppPath
	}
	m.flash = ui.Success.Render(msg.done)
	return m, nil
}

func (m Model) showLog(p pulls.PR) (tea.Model, tea.Cmd) {
	if len(p.FailingChecks()) == 0 {
		m.flash = ui.Muted.Render(p.Ref() + " has no failed checks")
		return m, nil
	}
	m.logs = newLogView(p, m.width-2*ui.HorizontalMargin, m.height)
	m.state = stateViewingLog
	return m, tea.Batch(m.readLog(m.logs.check()), m.startTicking())
}

func (m Model) readLog(check pulls.Check) tea.Cmd {
	return func() tea.Msg {
		lines, err := m.cfg.FailedLog(m.ctx, check)
		return logMsg{check: check, lines: lines, err: err}
	}
}

func (m Model) logLoaded(msg logMsg) (tea.Model, tea.Cmd) {
	if m.state != stateViewingLog || msg.check != m.logs.check() {
		return m, nil
	}
	m.logs.show(msg.lines, msg.err)
	return m, nil
}

func (m Model) updateViewingLog(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc", "q":
		m.state = stateListing
		return m, nil
	case "o":
		check := m.logs.check()
		m.state = stateListing
		return m.openInBrowser(check.URL, check.Name)
	case "tab":
		if m.logs.next() {
			return m, tea.Batch(m.readLog(m.logs.check()), m.startTicking())
		}
		return m, nil
	}
	var cmd tea.Cmd
	m.logs, cmd = m.logs.update(key)
	return m, cmd
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Reading your pull requests from GitHub…")
	case stateListing:
		content = m.listView()
	case stateChoosingIDE:
		content = m.picker.View(filepath.Base(m.pickingFor))
	case stateViewingLog:
		content = m.logs.view(m.spinner.View())
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt prs"
	v.AltScreen = true
	return v
}

func (m Model) listView() string {
	width := m.width - 2*ui.HorizontalMargin
	p, ok := m.list.current()
	clone, _ := m.clone(p)
	lines := []string{
		m.title(),
		"",
		m.list.view(),
		details(p, ok, clone, m.cfg.Home, m.list.now, width),
		m.help(),
	}
	return strings.Join(lines, "\n")
}

func (m Model) title() string {
	counts := fmt.Sprintf("  @%s%s%d to review%s%d open", m.list.viewer, noteJoiner, len(m.list.groups[0].prs), noteJoiner, len(m.list.groups[1].prs))
	title := ui.Title.Render("Pull requests") + ui.Muted.Render(counts)
	if m.refreshing {
		title += ui.Muted.Render(noteJoiner) + m.spinner.View() + ui.Muted.Render(" refreshing")
	}
	return title
}

// help is the key hints, after the latest action's result when there is one, cut to the width.
func (m Model) help() string {
	width := m.width - 2*ui.HorizontalMargin
	if m.flash == "" {
		return ui.Help.Render(ui.FitLine(listHelp, width))
	}
	flash := m.flash
	if m.working {
		flash = m.spinner.View() + " " + flash
	}
	return ui.Help.Render(ui.FitLine(flash+flashGap+ui.Muted.Render(listHelp), width))
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}
