// Package tui is the interactive terminal interface for `ngt standup`.
package tui

import (
	"context"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/standup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 110
	defaultHeight = 30
	// chromeLines is the space taken by everything around the report: the top margin, title,
	// blank line, and help line with its margin.
	chromeLines = 5
	help        = "↑/↓ move · enter/o open on GitHub · [ a day earlier · ] a day later · r refresh · q quit"
	flashGap    = "   "
)

type state int

const (
	stateLoading state = iota
	stateShowing
	stateDone
)

// Config holds where the projects are and how to read and open what you did.
type Config struct {
	Root string
	Home string
	// Since is the start of the report; zero means the last day you worked.
	Since time.Time
	// Load gathers the report since since, or since the last day you worked when since is zero.
	Load    func(ctx context.Context, since time.Time) (standup.Report, error)
	OpenURL func(url string) error
	Now     func() time.Time
}

// loadedMsg is a report, tagged with the load that asked for it so a slower earlier one is dropped.
type loadedMsg struct {
	seq    int
	report standup.Report
	err    error
}

type urlOpenedMsg struct {
	err error
}

// Model is the Bubble Tea model driving the standup screen.
type Model struct {
	ctx    context.Context
	cancel context.CancelFunc
	cfg    Config

	state   state
	width   int
	height  int
	spinner spinner.Model
	ticking bool
	loading bool
	seq     int
	now     time.Time
	report  standup.Report
	rows    []row
	cursor  ui.ListCursor
	// requested is the start asked for by the load in flight, zero for the last day you worked.
	requested time.Time
	darkBG    bool
	flash     string
	err       error
}

// New creates the model. Cancelling ctx stops loading.
func New(ctx context.Context, cfg Config) Model {
	ctx, cancel := context.WithCancel(ctx)
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return Model{
		ctx:     ctx,
		cancel:  cancel,
		cfg:     cfg,
		width:   defaultWidth,
		height:  defaultHeight,
		darkBG:  true,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points), spinner.WithStyle(ui.Title)),
		ticking: true,
		loading: true,
		seq:     1,
		now:     cfg.Now(),
	}
}

// Err is the error that ended the screen, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts loading.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.load(m.cfg.Since), tea.RequestBackgroundColor)
}

func (m Model) load(since time.Time) tea.Cmd {
	seq := m.seq
	return func() tea.Msg {
		report, err := m.cfg.Load(m.ctx, since)
		return loadedMsg{seq: seq, report: report, err: err}
	}
}

// Update routes messages to their handlers.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.cursor.Resize(m.listHeight())
		return m, nil
	case tea.BackgroundColorMsg:
		m.darkBG = msg.IsDark()
		return m, nil
	case spinner.TickMsg:
		return m.tick(msg)
	case loadedMsg:
		return m.loaded(msg)
	case urlOpenedMsg:
		if msg.err != nil {
			m.flash = ui.Warning.Render("Could not open it: " + msg.err.Error())
		}
		return m, nil
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m.quit()
		}
		if m.state == stateShowing {
			return m.handleKey(msg)
		}
		if msg.String() == "q" || msg.String() == "esc" {
			return m.quit()
		}
	}
	return m, nil
}

func (m Model) listHeight() int {
	return max(m.height-chromeLines, 1)
}

// tick animates the spinner while loading, and lets it stop otherwise.
func (m Model) tick(msg spinner.TickMsg) (tea.Model, tea.Cmd) {
	if !m.loading {
		m.ticking = false
		return m, nil
	}
	var cmd tea.Cmd
	m.spinner, cmd = m.spinner.Update(msg)
	return m, cmd
}

func (m Model) loaded(msg loadedMsg) (tea.Model, tea.Cmd) {
	if msg.seq != m.seq {
		return m, nil
	}
	m.loading = false
	if msg.err != nil {
		if m.state == stateLoading {
			m.err = msg.err
			return m.quit()
		}
		m.flash = ui.Warning.Render("Could not load: " + msg.err.Error())
		return m, nil
	}
	m.report = msg.report
	m.now = m.cfg.Now()
	m.rows = buildRows(m.report, m.now)
	m.cursor = ui.NewListCursor(len(m.rows), 0)
	m.cursor.Resize(m.listHeight())
	m.settle(1)
	m.state = stateShowing
	return m, nil
}

func (m Model) handleKey(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch key.String() {
	case "q", "esc":
		return m.quit()
	case "enter", "o":
		return m.open()
	case "t":
		return m.openTicket()
	case "[":
		return m.reload(m.shown().AddDate(0, 0, -1))
	case "]":
		return m.later()
	case "r":
		return m.reload(m.sinceForRefresh())
	}
	before := m.cursor.Index
	if m.cursor.HandleKey(key.String()) {
		m.settle(m.cursor.Index - before)
	}
	return m, nil
}

// settle moves the cursor off a blank line or a section title, onward in the direction it went,
// or back when there is nothing selectable that way.
func (m *Model) settle(direction int) {
	step := 1
	if direction < 0 {
		step = -1
	}
	for _, s := range []int{step, -step} {
		for i := m.cursor.Index; i >= 0 && i < len(m.rows); i += s {
			if m.rows[i].selectable() {
				m.cursor.MoveTo(i)
				return
			}
		}
	}
}

// shown is the start the screen is showing or about to show, so pressing [ twice goes back two days.
func (m Model) shown() time.Time {
	if m.loading && !m.requested.IsZero() {
		return m.requested
	}
	return m.report.Since
}

func (m Model) later() (tea.Model, tea.Cmd) {
	next := m.shown().AddDate(0, 0, 1)
	if next.After(m.cfg.Now()) {
		m.flash = ui.Muted.Render("The report already starts today")
		return m, nil
	}
	return m.reload(next)
}

// sinceForRefresh keeps the start the screen shows, unless it was worked out as the last day you
// worked, which a refresh works out again.
func (m Model) sinceForRefresh() time.Time {
	if m.report.LastWorkedDay {
		return time.Time{}
	}
	return m.report.Since
}

func (m Model) reload(since time.Time) (tea.Model, tea.Cmd) {
	m.seq++
	m.loading = true
	m.requested = since
	cmds := []tea.Cmd{m.load(since)}
	if !m.ticking {
		m.ticking = true
		cmds = append(cmds, m.spinner.Tick)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) open() (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, nil
	}
	r := m.rows[m.cursor.Index]
	if r.url == "" {
		m.flash = ui.Muted.Render(noURLReason(r))
		return m, nil
	}
	m.flash = ui.Muted.Render("Opening " + r.what + " on GitHub…")
	return m, func() tea.Msg { return urlOpenedMsg{err: m.cfg.OpenURL(r.url)} }
}

// openTicket opens the page of the ticket named in the selected row's title, subject or branch.
func (m Model) openTicket() (tea.Model, tea.Cmd) {
	if len(m.rows) == 0 {
		return m, nil
	}
	r := m.rows[m.cursor.Index]
	ticket, ok := ui.FindTicket(r.left.Text())
	if !ok {
		m.flash = ui.Muted.Render(ui.NoTicketReason(r.what))
		return m, nil
	}
	m.flash = ui.Muted.Render("Opening " + ticket.Key + " in your browser…")
	return m, func() tea.Msg { return urlOpenedMsg{err: m.cfg.OpenURL(ticket.URL)} }
}

func noURLReason(r row) string {
	switch r.kind {
	case inProgressRow:
		return "Work in progress is not on GitHub yet"
	case commitRow, groupRow:
		return r.what + " is not on GitHub: it is not pushed, or the repository has no GitHub remote"
	}
	return r.what + " is not on GitHub"
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Gathering what you did in "+ui.TildePath(m.cfg.Root, m.cfg.Home)+" and on GitHub…")
	case stateShowing:
		content = m.reportView()
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt standup"
	v.AltScreen = true
	return v
}

func (m Model) reportView() string {
	width := m.width - 2*ui.HorizontalMargin
	lines := []string{ui.FitLine(m.title(), width), ""}
	if m.report.Empty() {
		lines = append(lines, ui.Muted.Render(emptyMessage(m.report, m.now)))
	} else {
		lines = append(lines, m.rowsView(width))
	}
	lines = append(lines, m.help(width))
	return strings.Join(lines, "\n")
}

func (m Model) title() string {
	title := ui.Title.Render("Standup")
	if m.loading {
		title += " " + m.spinner.View() + ui.Muted.Render(" loading")
	}
	title += ui.Muted.Render("  since " + sinceLabel(m.report, m.now) + noteJoiner + summary(m.report))
	if m.report.GitHubErr != nil {
		title += ui.Muted.Render(noteJoiner) + ui.Warning.Render("GitHub not read: "+m.report.GitHubErr.Error())
	}
	return title
}

func (m Model) rowsView(width int) string {
	start, end := m.cursor.Visible()
	highlight := ui.HighlightColor(m.darkBG)
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		lines = append(lines, renderRow(m.rows[i], i == m.cursor.Index, highlight, width))
	}
	return strings.Join(lines, "\n")
}

// help is the key hints, after the latest action's result when there is one, cut to the width.
func (m Model) help(width int) string {
	keys := ui.WithTicketHelp(help, "t")
	line := keys
	if m.flash != "" {
		line = m.flash + flashGap + ui.Muted.Render(keys)
	}
	return ui.Help.Render(ui.FitLine(line, width))
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}
