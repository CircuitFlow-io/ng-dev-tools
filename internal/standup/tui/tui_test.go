package tui

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/standup"
)

// now is Tuesday 29 September 2026, 18:00.
var now = time.Date(2026, time.September, 29, 18, 0, 0, 0, time.UTC)

func sampleReport() standup.Report {
	since := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.UTC)
	merged := standup.PR{
		Repo: "acme/api", Number: 5, Title: "Add todo", URL: "https://github.com/acme/api/pull/5",
		State: standup.Merged, CreatedAt: since.Add(time.Hour), MergedAt: now.Add(-time.Hour),
	}
	return standup.Report{
		Since:         since,
		LastWorkedDay: true,
		Projects: []standup.Project{{
			Name: "api", Dir: "/p/api", URL: "https://github.com/acme/api",
			Groups: []standup.Group{
				{PR: &merged, Branch: "feat/todo", Commits: []standup.Commit{
					{Hash: "aaaaaaaaaa", Subject: "Add todo", At: now.Add(-2 * time.Hour), URL: "https://github.com/acme/api/commit/aaaaaaaaaa"},
				}},
				{Branch: "spike", DefaultBranch: "main", Commits: []standup.Commit{
					{Hash: "bbbbbbbbbb", Subject: "Try things", At: since.Add(20 * time.Hour)},
				}},
			},
		}},
		Reviewed: []standup.Reviewed{{Repo: "acme/web", Number: 9, Title: "Fix login", URL: "https://github.com/acme/web/pull/9", Author: "kim", Verdict: standup.Approved}},
		InProgress: []gitstatus.Repo{{
			Name: "api", Branch: "spike", HasRemote: true, Upstream: "origin/spike", Unpushed: 1,
			Files: []gitstatus.File{{Kind: gitstatus.Untracked, Path: "notes.md"}},
		}},
	}
}

// fakeGitHub records what the screen asks to load and open.
type fakeGitHub struct {
	mu     sync.Mutex
	since  []time.Time
	urls   []string
	report standup.Report
}

func (f *fakeGitHub) load(_ context.Context, since time.Time) (standup.Report, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.since = append(f.since, since)
	r := f.report
	if !since.IsZero() {
		r.Since, r.LastWorkedDay = since, false
	}
	return r, nil
}

func (f *fakeGitHub) openURL(url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	return nil
}

func shown(t *testing.T, fake *fakeGitHub) Model {
	t.Helper()
	fake.report = sampleReport()
	cfg := Config{Root: "/p", Home: "/home", Load: fake.load, OpenURL: fake.openURL, Now: func() time.Time { return now }}
	m := New(context.Background(), cfg)
	next, _ := m.Update(tea.WindowSizeMsg{Width: 130, Height: 30})
	next, _ = next.(Model).Update(loadedMsg{seq: 1, report: fake.report})
	return next.(Model)
}

func press(t *testing.T, m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func run(cmd tea.Cmd) tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		for _, c := range batch {
			if m, ok := c().(loadedMsg); ok {
				return m
			}
		}
		return nil
	}
	return msg
}

func screen(m Model) string {
	return ansi.Strip(m.View().Content)
}

func TestReportShowsProjectsReviewsAndWorkInProgress(t *testing.T) {
	m := shown(t, &fakeGitHub{})
	out := screen(m)
	for _, want := range []string{
		"since Mon 28 Sep, the last day you worked · 2 commits in 1 repo · 1 PR opened · 1 merged · 1 reviewed",
		"#5 Add todo", "opened · merged", "aaaaaaa Add todo", "16:00",
		"spike", "not pushed", "bbbbbbb Try things", "Mon 20:00",
		"Reviewed", "acme/web#9 Fix login by kim", "approved",
		"In progress", "1 untracked · 1 not pushed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("screen lacks %q:\n%s", want, out)
		}
	}
}

func TestCursorSkipsBlankLinesAndSectionTitles(t *testing.T) {
	m := shown(t, &fakeGitHub{})
	var kinds []rowKind
	for range 8 {
		kinds = append(kinds, m.rows[m.cursor.Index].kind)
		m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	want := []rowKind{projectRow, groupRow, commitRow, groupRow, commitRow, reviewedRow, inProgressRow, inProgressRow}
	if !slices.Equal(kinds, want) {
		t.Errorf("rows visited = %v, want %v", kinds, want)
	}
	m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyUp})
	if m.rows[m.cursor.Index].kind != reviewedRow {
		t.Errorf("up from the last row landed on %v", m.rows[m.cursor.Index].kind)
	}
}

func TestOpenGoesToGitHubOrSaysWhyNot(t *testing.T) {
	fake := &fakeGitHub{}
	m := shown(t, fake)
	m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	m, cmd := press(t, m, tea.KeyPressMsg{Code: 'o', Text: "o"})
	run(cmd)
	if !slices.Equal(fake.urls, []string{"https://github.com/acme/api/pull/5"}) {
		t.Errorf("opened %q", fake.urls)
	}
	for range 3 {
		m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyDown})
	}
	m, cmd = press(t, m, tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil || !strings.Contains(screen(m), "bbbbbbb is not on GitHub") {
		t.Errorf("an unpushed commit should explain itself:\n%s", screen(m))
	}
}

func TestBracketsMoveTheStartByADay(t *testing.T) {
	fake := &fakeGitHub{}
	m := shown(t, fake)
	m, cmd := press(t, m, tea.KeyPressMsg{Code: '[', Text: "["})
	m, cmd2 := press(t, m, tea.KeyPressMsg{Code: '[', Text: "["})
	if !strings.Contains(screen(m), "loading") {
		t.Errorf("no loading sign:\n%s", screen(m))
	}
	stale := run(cmd).(loadedMsg)
	next, _ := m.Update(stale)
	m = next.(Model)
	if !m.loading {
		t.Error("an earlier load ended the newer one")
	}
	next, _ = m.Update(run(cmd2))
	m = next.(Model)
	want := []time.Time{time.Date(2026, time.September, 27, 0, 0, 0, 0, time.UTC), time.Date(2026, time.September, 26, 0, 0, 0, 0, time.UTC)}
	if !slices.EqualFunc(fake.since, want, time.Time.Equal) {
		t.Errorf("loaded since %v, want %v", fake.since, want)
	}
	if !strings.Contains(screen(m), "since Sat 26 Sep ·") {
		t.Errorf("title does not show the new start:\n%s", screen(m))
	}
}

func TestLaterStopsAtToday(t *testing.T) {
	fake := &fakeGitHub{}
	m := shown(t, fake)
	m, cmd := press(t, m, tea.KeyPressMsg{Code: ']', Text: "]"})
	next, _ := m.Update(run(cmd))
	m = next.(Model)
	m, cmd = press(t, m, tea.KeyPressMsg{Code: ']', Text: "]"})
	if cmd != nil || !strings.Contains(screen(m), "already starts today") {
		t.Errorf("went past today:\n%s", screen(m))
	}
}

func TestRefreshWorksOutTheLastDayAgain(t *testing.T) {
	fake := &fakeGitHub{}
	m := shown(t, fake)
	_, cmd := press(t, m, tea.KeyPressMsg{Code: 'r', Text: "r"})
	run(cmd)
	if len(fake.since) != 1 || !fake.since[0].IsZero() {
		t.Errorf("refresh loaded since %v, want the last day you worked", fake.since)
	}
}

func TestFirstLoadErrorEndsTheScreen(t *testing.T) {
	m := New(context.Background(), Config{Load: (&fakeGitHub{}).load})
	next, cmd := m.Update(loadedMsg{seq: 1, err: errors.New("no projects")})
	if next.(Model).Err() == nil || cmd == nil {
		t.Error("the error did not end the screen")
	}
}

func TestPlainLines(t *testing.T) {
	lines := PlainLines(sampleReport(), now)
	want := []string{
		"Standup since Mon 28 Sep, the last day you worked · 2 commits in 1 repo · 1 PR opened · 1 merged · 1 reviewed",
		"",
		"api  (2 commits · 1 PR)",
		"  #5 Add todo  (opened · merged)",
		"    aaaaaaa Add todo  (16:00)",
		"  spike  (not pushed)",
		"    bbbbbbb Try things  (Mon 20:00)",
		"",
		"Reviewed",
		"  acme/web#9 Fix login by kim  (approved)",
		"",
		"In progress",
		"  api  spike  (1 untracked · 1 not pushed)",
	}
	if !slices.Equal(lines, want) {
		t.Errorf("lines:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
	empty := PlainLines(standup.Report{Since: time.Date(2026, time.September, 29, 0, 0, 0, 0, time.UTC)}, now)
	if empty[len(empty)-1] != "Nothing since today: no commits, pull requests or reviews" {
		t.Errorf("empty report = %q", empty)
	}
}
