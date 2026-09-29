package tui

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nasserghiasi/ng-dev-tools/internal/gitstatus"
	"github.com/nasserghiasi/ng-dev-tools/internal/ide"
	"github.com/nasserghiasi/ng-dev-tools/internal/macos/macostest"
)

var (
	zed    = ide.IDE{Name: "Zed", Version: "1.21", AppPath: "/Applications/Zed.app"}
	cursor = ide.IDE{Name: "Cursor", Version: "3.22", AppPath: "/Applications/Cursor.app"}
)

func sampleRepos() []gitstatus.Repo {
	now := time.Now()
	return []gitstatus.Repo{
		{Name: "trip-planner", Path: "/p/trip-planner", Branch: "main", HasRemote: true, Upstream: "origin/main", LastCommit: gitstatus.Commit{At: now.Add(-48 * time.Hour)}},
		{
			Name: "memorit", Path: "/p/memorit", Branch: "chore/upgrade", HasRemote: true, Unpushed: 2,
			Files:      []gitstatus.File{{Path: "apps/web/next.config.ts", Staged: '.', Unstaged: 'M'}},
			Stashes:    []gitstatus.Stash{{Ref: "stash@{0}", At: now, Message: "WIP on main"}},
			LastCommit: gitstatus.Commit{At: now.Add(-time.Hour)},
		},
		{Name: "ng-dev-tools", Path: "/p/ng-dev-tools", Branch: "main", Unborn: true, Files: []gitstatus.File{{Path: "go.mod", Kind: gitstatus.Untracked}}},
		{
			Name: "rebasing", Path: "/p/rebasing", Branch: "fix", HasRemote: true, Upstream: "origin/fix", Operation: gitstatus.Rebase,
			Files: []gitstatus.File{{Path: "api.go", Staged: 'U', Unstaged: 'U', Kind: gitstatus.Unmerged}},
		},
	}
}

// fakeActions records the fetches and opens the screen asks for.
type fakeActions struct {
	mu       sync.Mutex
	fetched  []string
	fetchErr error
	opened   []string
}

func (f *fakeActions) fetch(_ context.Context, dir string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fetched = append(f.fetched, dir)
	return f.fetchErr
}

func (f *fakeActions) open(path string, editor ide.IDE) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, path+" in "+editor.Name)
	return nil
}

func loaded(t *testing.T, cfg Config, actions *fakeActions) (Model, tea.Cmd) {
	t.Helper()
	cfg.Root, cfg.Home = "/p", "/home"
	cfg.Runner = &macostest.Runner{}
	cfg.Fetch, cfg.Open = actions.fetch, actions.open
	next, cmd := New(context.Background(), cfg).Update(loadedMsg{repos: sampleRepos()})
	return next.(Model), cmd
}

func press(t *testing.T, m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

// deliver runs cmd, following batches, and feeds every message but spinner ticks back to m.
func deliver(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			m = deliver(t, m, c)
		}
	case fetchedMsg, openedMsg, loadedMsg:
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func key(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(text[0]), Text: text}
}

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

func rowOf(t *testing.T, m Model, name string) string {
	t.Helper()
	for line := range strings.Lines(view(m)) {
		if strings.Contains(line, " "+name+" ") && !strings.Contains(line, "│") {
			return line
		}
	}
	t.Fatalf("no row for %s in\n%s", name, view(m))
	return ""
}

func TestRowsAreSortedMostUrgentFirst(t *testing.T) {
	m, _ := loaded(t, Config{}, &fakeActions{})
	out := view(m)
	order := []string{"▸ ✖ rebasing", "● memorit", "● ng-dev-tools", "✓ trip-planner"}
	last := -1
	for _, want := range order {
		i := strings.Index(out, want)
		if i < 0 || i < last {
			t.Fatalf("%q is out of order in\n%s", want, out)
		}
		last = i
	}
	if !strings.Contains(out, "3 need attention") {
		t.Errorf("title should count the repos needing attention:\n%s", out)
	}
	for name, want := range map[string]string{"memorit": "⇡2 not pushed", "ng-dev-tools": "no remote", "trip-planner": "clean"} {
		if row := rowOf(t, m, name); !strings.Contains(row, want) {
			t.Errorf("%s row %q lacks %q", name, row, want)
		}
	}
}

func TestDetailsPutAnOperationFirst(t *testing.T) {
	m, _ := loaded(t, Config{}, &fakeActions{})
	out := view(m)
	operation := strings.Index(out, "Rebase in progress: finish with git rebase --continue or --abort")
	changes := strings.Index(out, "CHANGES  1 conflicted")
	if operation < 0 || changes < 0 || operation > changes {
		t.Errorf("the rebase should be explained before the changes:\n%s", out)
	}
}

func TestDetailsListFilesAndStashes(t *testing.T) {
	m, _ := loaded(t, Config{}, &fakeActions{})
	m, _ = press(t, m, down)
	out := view(m)
	for _, want := range []string{"memorit  chore/upgrade · not pushed yet", " M apps/web/next.config.ts", "NOT PUSHED  2 commits", "stash@{0}"} {
		if !strings.Contains(out, want) {
			t.Errorf("details lack %q:\n%s", want, out)
		}
	}
}

func TestEnterOpensTheIDEBoxOnTheProjectsIDE(t *testing.T) {
	actions := &fakeActions{}
	cfg := Config{IDEs: []ide.IDE{cursor, zed}, ProjectIDEs: map[string]string{"/p/rebasing": zed.AppPath}}
	m, _ := loaded(t, cfg, actions)
	m, _ = press(t, m, enter)
	if m.state != stateChoosingIDE || m.picker.Current() != zed {
		t.Fatalf("state %v preselected %v, want the IDE box on Zed", m.state, m.picker.Current())
	}
	m, cmd := press(t, m, enter)
	m = deliver(t, m, cmd)
	if len(actions.opened) != 1 || actions.opened[0] != "/p/rebasing in Zed" {
		t.Errorf("opened = %q", actions.opened)
	}
	if m.state != stateListing || !strings.Contains(view(m), "Opened rebasing in Zed") {
		t.Errorf("the list should come back with a confirmation:\n%s", view(m))
	}
}

func TestEscLeavesTheIDEBoxWithoutOpening(t *testing.T) {
	actions := &fakeActions{}
	m, _ := loaded(t, Config{IDEs: []ide.IDE{cursor, zed}}, actions)
	m, _ = press(t, m, enter)
	m, _ = press(t, m, esc)
	if m.state != stateListing || len(actions.opened) != 0 {
		t.Errorf("state %v opened %q", m.state, actions.opened)
	}
}

func TestSingleIDEOpensStraightAway(t *testing.T) {
	actions := &fakeActions{}
	m, _ := loaded(t, Config{IDEs: []ide.IDE{zed}}, actions)
	m, cmd := press(t, m, enter)
	deliver(t, m, cmd)
	if len(actions.opened) != 1 {
		t.Errorf("opened = %q, want one project without a box", actions.opened)
	}
}

func TestFetchShowsProgressThenTheFailure(t *testing.T) {
	actions := &fakeActions{fetchErr: errors.New("Permission denied (publickey)")}
	m, _ := loaded(t, Config{}, actions)
	m, cmd := press(t, m, key("f"))
	if row := rowOf(t, m, "rebasing"); !strings.Contains(row, "fetching") {
		t.Fatalf("row while fetching = %q", row)
	}
	m = deliver(t, m, cmd)
	if len(actions.fetched) != 1 || actions.fetched[0] != "/p/rebasing" {
		t.Errorf("fetched = %q", actions.fetched)
	}
	if m.table.anyFetching() || !strings.Contains(view(m), "fetch failed") || !strings.Contains(view(m), "Fetch failed: Permission denied (publickey)") {
		t.Errorf("the failure should show in the row and the details:\n%s", view(m))
	}
}

func TestFetchOnStartSkipsReposWithoutARemote(t *testing.T) {
	actions := &fakeActions{}
	m, cmd := loaded(t, Config{FetchOnStart: true}, actions)
	if !strings.Contains(view(m), "fetching 3…") {
		t.Errorf("three repos have a remote:\n%s", view(m))
	}
	deliver(t, m, cmd)
	if len(actions.fetched) != 3 {
		t.Errorf("fetched = %q", actions.fetched)
	}
}

func TestFetchWithoutARemoteExplainsWhy(t *testing.T) {
	actions := &fakeActions{}
	m, _ := loaded(t, Config{}, actions)
	for range 2 {
		m, _ = press(t, m, down)
	}
	m, _ = press(t, m, key("f"))
	if len(actions.fetched) != 0 || !strings.Contains(view(m), "ng-dev-tools has no remote to fetch from") {
		t.Errorf("fetched %q:\n%s", actions.fetched, view(m))
	}
}

func TestNarrowTerminalDropsTheNotes(t *testing.T) {
	m, _ := loaded(t, Config{}, &fakeActions{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	if strings.Contains(view(next.(Model)), "NOTES") {
		t.Errorf("a 70-column terminal has no room for notes:\n%s", view(next.(Model)))
	}
}

func TestNoRepositoriesIsAnError(t *testing.T) {
	cfg := Config{Root: "/p", Home: "/home"}
	next, _ := New(context.Background(), cfg).Update(loadedMsg{})
	if err := next.(Model).Err(); err == nil || !strings.Contains(err.Error(), "no git repositories in /p") {
		t.Errorf("Err() = %v", err)
	}
}

func TestQuit(t *testing.T) {
	m, _ := loaded(t, Config{}, &fakeActions{})
	m, cmd := press(t, m, key("q"))
	if m.state != stateDone || cmd == nil {
		t.Error("q should quit")
	}
}

func TestPlainRow(t *testing.T) {
	r := sampleRepos()[1]
	got := PlainRow(r, time.Now(), "")
	want := []string{"memorit", "chore/upgrade", "~1", "⇡2 not pushed", "1h 0m ago", "1 stash"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("PlainRow = %q, want %q", got, want)
	}
	if failed := PlainRow(r, time.Now(), "denied"); failed[3] != "fetch failed" {
		t.Errorf("sync with a failed fetch = %q", failed[3])
	}
}
