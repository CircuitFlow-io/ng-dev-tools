package tui

import (
	"image/color"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	cursorWidth    = 2
	glyphWidth     = 2
	columnGap      = 2
	minNameWidth   = 10
	maxNameWidth   = 28
	minBranchWidth = 8
	maxBranchWidth = 32
	// preferredBranchWidth is how much of the branch stays before the notes start taking room from it.
	preferredBranchWidth = 24
	minNotesWidth        = 14
	maxNotesWidth        = 40
	// fetchingWidth keeps the sync column wide enough for "∙∙∙ fetching", so it does not jump.
	fetchingWidth = 12
)

var branchStyle = lipgloss.NewStyle().Foreground(ui.ColorAccent)

// table is the list of repositories with a cursor, and what is known about their fetches.
type table struct {
	repos  []gitstatus.Repo
	cursor ui.ListCursor
	// fetching holds the paths being fetched, fetchErrs why the last fetch of a path failed.
	fetching  map[string]bool
	fetchErrs map[string]string
	width     int
	height    int
	now       time.Time
	spinner   string
	highlight color.Color
}

func newTable() table {
	return table{fetching: map[string]bool{}, fetchErrs: map[string]string{}, height: 1, now: time.Now()}
}

// setRepos replaces the rows, sorted most urgent first, keeping the cursor on the same repository.
func (t *table) setRepos(repos []gitstatus.Repo) {
	current, hadCurrent := t.current()
	gitstatus.Sort(repos)
	t.repos = repos
	t.now = time.Now()
	index := 0
	if hadCurrent {
		index = max(0, slices.IndexFunc(repos, func(r gitstatus.Repo) bool { return r.Path == current.Path }))
	}
	t.cursor = ui.NewListCursor(len(repos), index)
	t.cursor.Resize(t.height)
}

// replace swaps in a freshly read repository.
func (t *table) replace(repo gitstatus.Repo) {
	repos := slices.Clone(t.repos)
	i := slices.IndexFunc(repos, func(r gitstatus.Repo) bool { return r.Path == repo.Path })
	if i < 0 {
		return
	}
	repos[i] = repo
	t.setRepos(repos)
}

func (t table) current() (gitstatus.Repo, bool) {
	if len(t.repos) == 0 {
		return gitstatus.Repo{}, false
	}
	return t.repos[t.cursor.Index], true
}

func (t *table) resize(width, height int) {
	t.width = width
	t.height = max(height, 1)
	t.cursor.Resize(t.height)
}

func (t table) needingAttention() int {
	n := 0
	for _, r := range t.repos {
		if r.Attention().NeedsAttention() {
			n++
		}
	}
	return n
}

func (t table) anyFetching() bool {
	return len(t.fetching) > 0
}

func (t table) syncState(r gitstatus.Repo) syncState {
	return syncState{fetching: t.fetching[r.Path], spinner: t.spinner, fetchErr: t.fetchErrs[r.Path]}
}

type layout struct {
	name, branch, changes, sync, lastCommit, notes int
}

// layout fits the columns to their content. When space runs out the branch gives way to the notes
// down to preferredBranchWidth; with less room still, the notes are left out and the branch takes
// what is left.
func (t table) layout() layout {
	l := layout{
		name:       t.fit("PROJECT", func(r gitstatus.Repo) cell { return cell{{r.Name, plain}} }, minNameWidth, maxNameWidth),
		changes:    t.fit("CHANGES", changesCell, 0, t.width),
		sync:       max(fetchingWidth+columnGap, t.fit("SYNC", func(r gitstatus.Repo) cell { return syncCell(r, t.syncState(r)) }, 0, t.width)),
		lastCommit: t.fit("LAST COMMIT", func(r gitstatus.Repo) cell { return lastCommitCell(r, t.now) }, 0, t.width),
	}
	branch := t.fit("BRANCH", func(r gitstatus.Repo) cell { return cell{{r.Branch, plain}} }, minBranchWidth, maxBranchWidth)
	notes := t.fit("NOTES", notesCell, minNotesWidth, maxNotesWidth)
	rest := t.width - cursorWidth - glyphWidth - l.name - l.changes - l.sync - l.lastCommit
	keptBranch := min(branch, preferredBranchWidth)
	if rest-minNotesWidth < keptBranch {
		l.branch = min(branch, max(minBranchWidth, rest))
		return l
	}
	l.branch = min(branch, max(keptBranch, rest-notes))
	l.notes = rest - l.branch
	return l
}

func (t table) fit(title string, field func(gitstatus.Repo) cell, least, most int) int {
	longest := lipgloss.Width(title)
	for _, r := range t.repos {
		longest = max(longest, field(r).width())
	}
	return min(most, max(least, longest+columnGap))
}

func (t table) header() string {
	l := t.layout()
	cells := strings.Repeat(" ", cursorWidth+glyphWidth) +
		ui.PadRight("PROJECT", l.name) + ui.PadRight("BRANCH", l.branch) + ui.PadRight("CHANGES", l.changes) +
		ui.PadRight("SYNC", l.sync) + ui.PadRight("LAST COMMIT", l.lastCommit)
	if l.notes > 0 {
		cells += "NOTES"
	}
	return ui.Muted.Render(cells)
}

// view renders the visible rows. A scrolling table is padded to its height, so what follows stays
// in place.
func (t table) view() string {
	l := t.layout()
	start, end := t.cursor.Visible()
	lines := make([]string, 0, t.height)
	for i := start; i < end; i++ {
		lines = append(lines, t.row(i, l))
	}
	for len(lines) < min(t.height, len(t.repos)) {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (t table) row(index int, l layout) string {
	r := t.repos[index]
	painter := ui.NewRowPainter(index == t.cursor.Index, t.highlight)
	nameStyle := plain
	if painter.IsHighlighted() {
		nameStyle = ui.Bold
	}
	row := painter.Cursor() +
		glyphCell(r).render(painter, glyphWidth) +
		cell{{ui.Truncate(r.Name, l.name-columnGap), nameStyle}}.render(painter, l.name) +
		cell{{ui.Truncate(r.Branch, l.branch-columnGap), branchStyle}}.render(painter, l.branch) +
		changesCell(r).render(painter, l.changes) +
		syncCell(r, t.syncState(r)).render(painter, l.sync) +
		lastCommitCell(r, t.now).render(painter, l.lastCommit)
	if l.notes > 0 {
		row += notesCell(r).render(painter, l.notes)
	}
	return painter.Fill(row, t.width)
}
