// Package tui is the interactive terminal interface for `ngt env`.
package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 110
	defaultHeight = 30
	// chromeLines is the space taken by everything around the table: the top margin, title, blank
	// line, column header, details box with its border, and help line with its margin.
	chromeLines = 6 + detailLines + 2
	listHelp    = "↑/↓ move · a add missing keys · r refresh · q quit"
	confirmHelp = "y add · n cancel"
	flashGap    = "   "
)

type state int

const (
	stateLoading state = iota
	stateListing
	stateConfirming
	stateDone
)

// Config holds where the projects are and how to read and change their env files.
type Config struct {
	Root string
	Home string
	Scan func(ctx context.Context) ([]envfiles.Set, error)
	// AddMissing appends a set's missing keys to its local file with empty values.
	AddMissing func(envfiles.Set) (file string, added int, err error)
}

type scannedMsg struct {
	sets []envfiles.Set
	err  error
}

type addedMsg struct {
	set   envfiles.Set
	file  string
	added int
	err   error
}

// Model is the Bubble Tea model driving the env screen.
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
	table      table
	flash      string
	now        time.Time
	err        error
}

// New creates the model. Cancelling ctx stops the scan.
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
		ticking: true,
		table:   newTable(),
		now:     time.Now(),
	}
	m.resize(m.width, m.height)
	return m
}

// Err is the error that ended the screen, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts scanning the projects.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.scan(), tea.RequestBackgroundColor)
}

func (m Model) scan() tea.Cmd {
	return func() tea.Msg {
		sets, err := m.cfg.Scan(m.ctx)
		return scannedMsg{sets: sets, err: err}
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
		return m, nil
	case spinner.TickMsg:
		return m.tick(msg)
	case scannedMsg:
		return m.showSets(msg)
	case addedMsg:
		return m.added(msg)
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
	case stateConfirming:
		return m.updateConfirming(key)
	}
	return m, nil
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.table.resize(width-2*ui.HorizontalMargin, height-chromeLines)
}

// tick animates the spinner while anything is loading, and lets it stop otherwise.
func (m Model) tick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if m.state != stateLoading && !m.refreshing {
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

func (m Model) showSets(msg scannedMsg) (tea.Model, tea.Cmd) {
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
	if firstLoad && len(msg.sets) == 0 {
		m.err = fmt.Errorf("no env files in %s", ui.TildePath(m.cfg.Root, m.cfg.Home))
		return m.quit()
	}
	m.now = time.Now()
	m.table.setSets(msg.sets)
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
	case "a":
		return m.confirmAdd()
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
	return m, tea.Batch(m.scan(), m.startTicking())
}

// confirmAdd asks before adding the missing keys of the set under the cursor.
func (m Model) confirmAdd() (tea.Model, tea.Cmd) {
	s, ok := m.table.current()
	if !ok {
		return m, nil
	}
	if len(s.Missing) == 0 {
		m.flash = ui.Muted.Render(noMissingReason(s))
		return m, nil
	}
	m.state = stateConfirming
	return m, nil
}

func noMissingReason(s envfiles.Set) string {
	switch {
	case s.Example == "":
		return s.Name + " has no example to take keys from"
	case len(s.Locals) == 0:
		return "Copy " + s.Example + " to " + s.Base() + " first: there is no local file to add keys to"
	}
	return s.Name + " has every key " + s.Example + " lists"
}

func (m Model) updateConfirming(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "y", "enter":
		m.state = stateListing
		return m.add()
	case "n", "esc", "q":
		m.state = stateListing
	}
	return m, nil
}

func (m Model) add() (tea.Model, tea.Cmd) {
	s, ok := m.table.current()
	if !ok {
		return m, nil
	}
	return m, func() tea.Msg {
		file, added, err := m.cfg.AddMissing(s)
		return addedMsg{set: s, file: file, added: added, err: err}
	}
}

func (m Model) added(msg addedMsg) (tea.Model, tea.Cmd) {
	path := filepath.Join(msg.set.Name, msg.file)
	switch {
	case msg.err != nil:
		m.flash = ui.Warning.Render("Could not add the keys: " + msg.err.Error())
	case msg.added == 0:
		m.flash = ui.Muted.Render(path + " already has every key")
	default:
		m.flash = ui.Success.Render("Added "+ui.Count(msg.added, "key")+" to ") + ui.Bold.Render(path) + ui.Success.Render(": fill in their values")
	}
	m.refreshing = true
	return m, tea.Batch(m.scan(), m.startTicking())
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Reading env files in "+ui.TildePath(m.cfg.Root, m.cfg.Home)+"…")
	case stateListing, stateConfirming:
		content = m.listView()
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt env"
	v.AltScreen = true
	return v
}

func (m Model) listView() string {
	width := m.width - 2*ui.HorizontalMargin
	s, ok := m.table.current()
	lines := []string{
		m.title(),
		"",
		m.table.header(),
		m.table.view(),
		details(s, ok, m.now, width),
		m.help(width),
	}
	return strings.Join(lines, "\n")
}

func (m Model) title() string {
	facts := ui.Count(m.table.folders(), "folder") + " in " + ui.TildePath(m.cfg.Root, m.cfg.Home)
	summary := ui.Success.Render("all complete")
	if n := m.table.needingAttention(); n > 0 {
		summary = ui.Warning.Render(fmt.Sprintf("%d need attention", n))
	}
	title := ui.Title.Render("Env") + ui.Muted.Render("  "+facts+noteJoiner) + summary
	if m.refreshing {
		title += ui.Muted.Render(noteJoiner) + m.spinner.View() + ui.Muted.Render(" refreshing")
	}
	return title
}

// help is the key hints, after the latest action's result or the pending question, cut to the width.
func (m Model) help(width int) string {
	line := listHelp
	switch {
	case m.state == stateConfirming:
		line = m.confirmQuestion() + flashGap + ui.Muted.Render(confirmHelp)
	case m.flash != "":
		line = m.flash + flashGap + ui.Muted.Render(listHelp)
	}
	return ui.Help.Render(ui.FitLine(line, width))
}

func (m Model) confirmQuestion() string {
	s, _ := m.table.current()
	target, _ := s.AddTarget()
	return ui.Warning.Render("Add "+ui.Count(len(s.Missing), "missing key")+" to ") +
		ui.Bold.Render(filepath.Join(s.Name, target)) + ui.Warning.Render(" with empty values?")
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}
