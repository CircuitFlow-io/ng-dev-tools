// Package tui is the interactive terminal interface for `ngt todo`.
package tui

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide/idepicker"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	defaultWidth  = 110
	defaultHeight = 30
	// chromeLines is the space taken by everything around the table: the top margin, title, blank
	// line, column header, details box with its border, and help line with its margin.
	chromeLines = 6 + detailLines + 2
	listHelp    = "↑/↓ move · enter open in IDE · o open commit · m mine only · r refresh · q quit"
	mineHelp    = "↑/↓ move · enter open in IDE · o open commit · m show all · r refresh · q quit"
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

// Config holds where the projects are and how to act on the comments found.
type Config struct {
	Root string
	Home string
	// Find finds the marker comments, with the repositories that could not be read by name.
	Find    func(ctx context.Context) ([]todos.Item, map[string]error, error)
	OpenURL func(url string) error
	IDEs    []ide.IDE
	// ProjectIDEs maps a project path to the IDE app it was last opened in; DefaultIDE is the one
	// picked most recently for any project.
	ProjectIDEs map[string]string
	DefaultIDE  string
	// OpenAt opens an item's file at its line in an IDE and remembers the choice.
	OpenAt func(item todos.Item, editor ide.IDE) error
}

type foundMsg struct {
	items []todos.Item
	errs  map[string]error
	err   error
}

type openedMsg struct {
	item   todos.Item
	editor ide.IDE
	err    error
}

type urlOpenedMsg struct {
	err error
}

// code is the source around an item, read when the cursor first reaches it.
type code struct {
	lines []todos.SourceLine
	err   error
}

// Model is the Bubble Tea model driving the todo screen.
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
	unreadable map[string]error
	code       map[string]code
	picker     idepicker.Picker
	pickingFor todos.Item
	flash      string
	err        error
}

// New creates the model. Cancelling ctx stops the search.
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
		table:   newTable(),
		code:    map[string]code{},
	}
	m.resize(m.width, m.height)
	return m
}

// Err is the error that ended the screen, if any.
func (m Model) Err() error {
	return m.err
}

// Init starts the search.
func (m Model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.find(), tea.RequestBackgroundColor)
}

func (m Model) find() tea.Cmd {
	return func() tea.Msg {
		items, errs, err := m.cfg.Find(m.ctx)
		return foundMsg{items: items, errs: errs, err: err}
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
	case foundMsg:
		return m.showItems(msg)
	case openedMsg:
		return m.opened(msg)
	case urlOpenedMsg:
		if msg.err != nil {
			m.flash = ui.Warning.Render("Could not open it: " + msg.err.Error())
		}
		return m, nil
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

// tick animates the spinner while searching, and lets it stop otherwise.
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

func (m Model) showItems(msg foundMsg) (tea.Model, tea.Cmd) {
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
	m.unreadable = msg.errs
	m.code = map[string]code{}
	m.table.setItems(msg.items)
	m.state = stateListing
	return m, nil
}

func (m Model) updateListing(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	m.flash = ""
	switch key.String() {
	case "q", "esc":
		return m.quit()
	case "enter":
		return m.chooseIDE()
	case "o":
		return m.openCommit()
	case "t":
		return m.openTicket()
	case "m":
		m.table.toggleMine()
		return m, nil
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
	return m, tea.Batch(m.find(), m.startTicking())
}

// openTicket opens the page of the ticket named in the selected note, or else in the subject of
// the commit that added it.
func (m Model) openTicket() (tea.Model, tea.Cmd) {
	item, ok := m.table.current()
	if !ok {
		return m, nil
	}
	ticket, ok := ui.FindTicket(item.Note, item.Subject)
	if !ok {
		m.flash = ui.Muted.Render(ui.NoTicketReason("this note or its commit"))
		return m, nil
	}
	m.flash = ui.Muted.Render("Opening " + ticket.Key + " in your browser…")
	return m, func() tea.Msg { return urlOpenedMsg{err: m.cfg.OpenURL(ticket.URL)} }
}

func (m Model) openCommit() (tea.Model, tea.Cmd) {
	item, ok := m.table.current()
	if !ok {
		return m, nil
	}
	if item.CommitURL == "" {
		m.flash = ui.Muted.Render(noCommitReason(item))
		return m, nil
	}
	m.flash = ui.Muted.Render("Opening " + item.ShortCommit() + " on GitHub…")
	return m, func() tea.Msg { return urlOpenedMsg{err: m.cfg.OpenURL(item.CommitURL)} }
}

func noCommitReason(item todos.Item) string {
	if item.Uncommitted {
		return "This line is not committed yet"
	}
	return item.ShortCommit() + " is not on GitHub: it is not pushed, or the repository has no GitHub remote"
}

// chooseIDE shows the IDE box for the item under the cursor, or opens it straight away when only
// one IDE is installed.
func (m Model) chooseIDE() (tea.Model, tea.Cmd) {
	item, ok := m.table.current()
	if !ok {
		return m, nil
	}
	switch len(m.cfg.IDEs) {
	case 0:
		m.flash = ui.Warning.Render(errNoIDE.Error())
		return m, nil
	case 1:
		return m.open(item, m.cfg.IDEs[0])
	}
	m.picker = idepicker.New(m.cfg.IDEs, m.cfg.ProjectIDEs[item.Dir], m.cfg.DefaultIDE)
	m.picker.SetHighlight(ui.HighlightColor(m.darkBG))
	m.pickingFor = item
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

func (m Model) open(item todos.Item, editor ide.IDE) (tea.Model, tea.Cmd) {
	m.flash = ui.Muted.Render("Opening " + item.Location() + " in " + editor.Name + "…")
	return m, func() tea.Msg {
		return openedMsg{item: item, editor: editor, err: m.cfg.OpenAt(item, editor)}
	}
}

func (m Model) opened(msg openedMsg) (tea.Model, tea.Cmd) {
	if msg.err != nil {
		m.flash = ui.Warning.Render(msg.err.Error())
		return m, nil
	}
	m.cfg.ProjectIDEs[msg.item.Dir] = msg.editor.AppPath
	m.cfg.DefaultIDE = msg.editor.AppPath
	m.flash = ui.Success.Render("Opened ") + ui.Bold.Render(msg.item.Location()) + ui.Success.Render(" in ") + ui.Bold.Render(msg.editor.Name)
	return m, nil
}

// View renders the current screen.
func (m Model) View() tea.View {
	var content string
	switch m.state {
	case stateLoading:
		content = m.spinner.View() + " " + ui.Title.Render("Looking for TODO, FIXME and HACK in "+ui.TildePath(m.cfg.Root, m.cfg.Home)+"…")
	case stateListing:
		content = m.listView()
	case stateChoosingIDE:
		content = m.picker.View(m.pickingFor.Project)
	case stateDone:
		return tea.NewView("")
	}
	v := tea.NewView(ui.Screen.Render(content))
	v.WindowTitle = "ngt todo"
	v.AltScreen = true
	return v
}

func (m Model) listView() string {
	width := m.width - 2*ui.HorizontalMargin
	item, ok := m.table.current()
	source := m.source(item, ok)
	lines := []string{
		ui.FitLine(m.title(), width),
		"",
		m.table.header(),
		m.table.view(),
		details(item, ok, source.lines, source.err, m.table.now, width),
		m.help(width),
	}
	return strings.Join(lines, "\n")
}

// source reads the code around item once, and keeps it for when the cursor comes back.
func (m Model) source(item todos.Item, ok bool) code {
	if !ok {
		return code{}
	}
	if c, seen := m.code[item.ID()]; seen {
		return c
	}
	lines, err := todos.Surroundings(item, linesAbove, linesBelow)
	c := code{lines: lines, err: err}
	m.code[item.ID()] = c
	return c
}

func (m Model) title() string {
	t := m.table
	facts := fmt.Sprintf("%s in %s", ui.Count(len(t.all), "comment"), ui.Count(t.repos(), "repo"))
	if t.mineOnly {
		facts = fmt.Sprintf("%d of %d are yours", len(t.items), len(t.all))
	} else if n := t.mine(); n > 0 {
		facts += fmt.Sprintf("%s%d yours", noteJoiner, n)
	}
	if oldest, ok := t.oldest(); ok {
		facts += noteJoiner + "oldest " + oldest
	}
	title := ui.Title.Render("Todo") + ui.Muted.Render("  "+facts)
	if n := len(m.unreadable); n > 0 {
		title += ui.Muted.Render(noteJoiner) + ui.Warning.Render(fmt.Sprintf("%s unreadable: %s", ui.Count(n, "repo"), strings.Join(sortedKeys(m.unreadable), ", ")))
	}
	if m.refreshing {
		title += ui.Muted.Render(noteJoiner) + m.spinner.View() + ui.Muted.Render(" refreshing")
	}
	return title
}

func (t table) oldest() (string, bool) {
	for _, i := range t.items {
		if !i.Uncommitted {
			return ui.Age(t.now, i.At), true
		}
	}
	return "", false
}

func sortedKeys(m map[string]error) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// help is the key hints, after the latest action's result when there is one, cut to the width.
func (m Model) help(width int) string {
	hints := listHelp
	if m.table.mineOnly {
		hints = mineHelp
	}
	hints = ui.WithTicketHelp(hints, "t")
	line := hints
	if m.flash != "" {
		line = m.flash + flashGap + ui.Muted.Render(hints)
	}
	return ui.Help.Render(ui.FitLine(line, width))
}

func (m Model) quit() (tea.Model, tea.Cmd) {
	m.cancel()
	m.state = stateDone
	return m, tea.Quit
}
