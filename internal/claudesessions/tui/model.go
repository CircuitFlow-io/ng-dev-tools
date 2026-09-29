// Package tui is the interactive terminal interface for `ngt claude sessions`.
package tui

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/projectlist"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 120
	defaultHeight = 30
	// chromeLines is the space taken by everything around the table: the top margin, title, filter
	// line, blank line, column header, details box with its border, and help line with its margin.
	chromeLines = 7 + detailLines + 2
	help        = "type to search · ↑/↓ move · enter resume · esc clear or quit"
	placeholder = "type to search prompts, replies, titles, folders and branches"
	flashGap    = "   "
)

type state int

const (
	stateLoading state = iota
	stateListing
	stateDone
)

// Config holds where the sessions are and how to read them.
type Config struct {
	// Dir is Claude Code's sessions folder, shown while reading it.
	Dir  string
	Home string
	// Root is the projects folder; sessions inside it are named by their path from it.
	Root  string
	Query string
	// Find reads the sessions, with the transcripts that could not be read by path.
	Find func(ctx context.Context) ([]claudesessions.Session, map[string]error, error)
}

type foundMsg struct {
	sessions []claudesessions.Session
	errs     map[string]error
	err      error
}

// Model is the Bubble Tea model driving the sessions screen.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	state      state
	width      int
	height     int
	spinner    spinner.Model
	index      claudesessions.Index
	folders    int
	unreadable int
	query      string
	table      table
	flash      string
	chosen     claudesessions.Session
	resume     bool
	err        error
}

// New creates the model. Cancelling ctx stops the reading.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	m := Model{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		width:   defaultWidth,
		height:  defaultHeight,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
		query:   cfg.Query,
		table:   newTable(cfg.Home, cfg.Root),
	}
	m.table.highlight = ui.HighlightColor(true)
	m.resize(m.width, m.height)
	return m
}

// Chosen is the session to resume, or ok false when the user quit.
func (m Model) Chosen() (session claudesessions.Session, ok bool) {
	return m.chosen, m.resume
}

// Err is the error that ended the screen, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts reading the sessions.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.find(), tea.RequestBackgroundColor)
}

func (m Model) find() tea.Cmd {
	return func() tea.Msg {
		sessions, errs, err := m.cfg.Find(m.ctx)
		return foundMsg{sessions: sessions, errs: errs, err: err}
	}
}

// Update routes messages to the handler for the current screen.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.BackgroundColorMsg:
		m.table.highlight = ui.HighlightColor(msg.IsDark())
		return m, nil
	case spinner.TickMsg:
		if m.state != stateLoading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	case foundMsg:
		return m.showSessions(msg)
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
		if m.state == stateListing {
			return m.updateListing(msg)
		}
	}
	return m, nil
}

func (m *Model) resize(width, height int) {
	m.width, m.height = width, height
	m.table.resize(width-2*ui.HorizontalMargin, height-chromeLines)
}

func (m Model) showSessions(msg foundMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.err = msg.err
		return m.quit()
	}
	if len(msg.sessions) == 0 {
		m.err = fmt.Errorf("no Claude Code sessions in %s", ui.TildePath(m.cfg.Dir, m.cfg.Home))
		return m.quit()
	}
	dirs := map[string]bool{}
	for _, s := range msg.sessions {
		if _, seen := dirs[s.Dir]; !seen {
			dirs[s.Dir] = true
			m.table.missing[s.Dir] = !s.DirExists()
		}
	}
	m.folders = len(dirs)
	m.unreadable = len(msg.errs)
	m.index = claudesessions.NewIndex(msg.sessions)
	m.search()
	m.state = stateListing
	return m, nil
}

func (m *Model) search() {
	m.table.setResults(m.index.Search(m.query))
}

func (m Model) updateListing(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch key.String() {
	case "enter":
		return m.choose()
	case "esc":
		if m.query == "" {
			return m.quit()
		}
		m.setQuery("")
		return m, nil
	case "backspace":
		m.setQuery(dropLastRune(m.query))
		return m, nil
	}
	if projectlist.IsTyping(key) {
		m.setQuery(m.query + key.Text)
		return m, nil
	}
	m.table.cursor.HandleKey(key.String())
	return m, nil
}

func (m *Model) setQuery(query string) {
	m.query = query
	m.search()
}

func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

// choose ends the screen with the session under the cursor, unless its folder is gone.
func (m Model) choose() (tea.Model, tea.Cmd) {
	result, ok := m.table.current()
	if !ok {
		return m, nil
	}
	s := result.Session
	if m.table.missing[s.Dir] {
		m.flash = ui.Warning.Render(ui.TildePath(s.Dir, m.cfg.Home) + " no longer exists, so claude cannot resume there")
		return m, nil
	}
	m.chosen, m.resume = s, true
	m.state = stateDone
	return m, tea.Quit
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Reading Claude sessions in "+ui.TildePath(m.cfg.Dir, m.cfg.Home)+"…")
	case stateListing:
		content = m.listView()
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt claude sessions"
	v.AltScreen = true
	return v
}

func (m Model) listView() string {
	width := m.width - 2*ui.HorizontalMargin
	result, ok := m.table.current()
	in := detailsInput{result: result, missing: m.table.missing[result.Session.Dir], now: m.table.now, home: m.cfg.Home}
	lines := []string{
		ui.FitLine(m.title(), width),
		ui.FitLine(projectlist.FilterLine(m.query, placeholder), width),
		"",
		m.table.header(),
		m.table.view(),
		details(in, ok, width),
		m.help(width),
	}
	return strings.Join(lines, "\n")
}

func (m Model) title() string {
	facts := fmt.Sprintf("%s in %s", ui.Count(m.index.Len(), "session"), ui.Count(m.folders, "folder"))
	if m.query != "" {
		facts = fmt.Sprintf("%d of %s match", len(m.table.results), ui.Count(m.index.Len(), "session"))
	}
	title := ui.Title.Render("Claude sessions") + ui.Muted.Render("  "+facts)
	if m.unreadable > 0 {
		title += ui.Muted.Render(joiner) + ui.Warning.Render(ui.Count(m.unreadable, "transcript")+" unreadable")
	}
	return title
}

// help is the key hints, after the latest action's result when there is one, cut to the width.
func (m Model) help(width int) string {
	line := help
	if m.flash != "" {
		line = m.flash + flashGap + ui.Muted.Render(help)
	}
	return ui.Help.Render(ui.FitLine(line, width))
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}
