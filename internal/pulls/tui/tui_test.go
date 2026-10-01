package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
)

var (
	zed    = ide.IDE{Name: "Zed", Version: "1.21", AppPath: "/Applications/Zed.app"}
	cursor = ide.IDE{Name: "Cursor", Version: "3.22", AppPath: "/Applications/Cursor.app"}
)

func sampleDashboard() pulls.Dashboard {
	now := time.Now()
	return pulls.Dashboard{
		Viewer: "me",
		ToReview: []pulls.PR{{
			Repo: "acme/web", Number: 9, Title: "Add search", URL: "https://github.com/acme/web/pull/9", Author: "kim",
			HeadRef: "search", BaseRef: "main", Mergeable: "CONFLICTING", ReviewDecision: pulls.ReviewRequired,
			UpdatedAt: now.Add(-time.Hour), Requested: []string{"me"},
		}},
		Mine: []pulls.PR{{
			Repo: "acme/api", Number: 2, Title: "Faster login", URL: "https://github.com/acme/api/pull/2", Author: "me",
			HeadRef: "fast-login", BaseRef: "main", Mergeable: "MERGEABLE", UpdatedAt: now.Add(-2 * time.Hour),
			Checks: []pulls.Check{
				{Name: "test", Workflow: "CI", URL: "https://github.com/acme/api/actions/runs/1/job/12", State: pulls.Failed},
				{Name: "e2e", Workflow: "CI", URL: "https://github.com/acme/api/actions/runs/1/job/13", State: pulls.Failed},
				{Name: "lint", Workflow: "CI", State: pulls.Passed},
			},
			Reviews: []pulls.Review{{Author: "sam", State: pulls.Approved}},
		}},
	}
}

// fakeActions records what the screen asks to do.
type fakeActions struct {
	mu        sync.Mutex
	opened    []string
	checkouts []string
	clones    []string
	ides      []string
	logs      []string
}

func (f *fakeActions) record(list *[]string, entry string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	*list = append(*list, entry)
}

func loaded(t *testing.T, cfg Config, actions *fakeActions) Model {
	t.Helper()
	cfg.Root, cfg.Home = "/p", "/home"
	cfg.OpenURL = func(url string) error { actions.record(&actions.opened, url); return nil }
	cfg.Clone = func(_ context.Context, repo string) (string, error) {
		actions.record(&actions.clones, repo)
		return "/p/" + repo[strings.Index(repo, "/")+1:], nil
	}
	cfg.Checkout = func(_ context.Context, dir string, p pulls.PR) error {
		actions.record(&actions.checkouts, dir+" "+p.Ref())
		return nil
	}
	cfg.Open = func(path string, editor ide.IDE) error {
		actions.record(&actions.ides, path+" in "+editor.Name)
		return nil
	}
	cfg.FailedLog = func(_ context.Context, check pulls.Check) ([]pulls.LogLine, error) {
		actions.record(&actions.logs, check.Name)
		return []pulls.LogLine{{Step: "Run tests", Text: "##[error]" + check.Name + " broke"}}, nil
	}
	clones := map[string]string{"acme/api": "/p/api"}
	next, _ := New(context.Background(), cfg).Update(loadedMsg{dashboard: sampleDashboard(), clones: clones})
	return next.(Model)
}

func press(t *testing.T, m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

// deliver runs cmd, following batches, and feeds the screen's own messages back to m.
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
	case actionMsg, logMsg, loadedMsg:
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
	tab   = tea.KeyPressMsg{Code: tea.KeyTab}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

func TestListGroupsReviewsBeforeYourOwn(t *testing.T) {
	m := loaded(t, Config{}, &fakeActions{})
	out := view(m)
	order := []string{"NEEDS YOUR REVIEW  1", "▸ ✖ web#9", "Add search  @kim", "conflicts", "YOURS  1", "✖ api#2", "checks failing", "✖ 2/3"}
	last := -1
	for _, want := range order {
		i := strings.Index(out, want)
		if i < 0 || i < last {
			t.Fatalf("%q missing or out of order in\n%s", want, out)
		}
		last = i
	}
	if !strings.Contains(out, "@me · 1 to review · 1 open") {
		t.Errorf("title should count both groups:\n%s", out)
	}
}

func TestEmptyGroupSaysSo(t *testing.T) {
	cfg := Config{Root: "/p", Home: "/home"}
	next, _ := New(context.Background(), cfg).Update(loadedMsg{dashboard: pulls.Dashboard{Viewer: "me"}})
	out := view(next.(Model))
	if !strings.Contains(out, "Nothing is waiting for your review") || !strings.Contains(out, "You have no open pull requests") {
		t.Errorf("empty groups should explain themselves:\n%s", out)
	}
}

func TestDetailsDescribeTheSelectedPullRequest(t *testing.T) {
	m := loaded(t, Config{}, &fakeActions{})
	out := view(m)
	for _, want := range []string{"acme/web#9  Add search", "@kim wants to merge search into main", "not cloned locally", "○ waiting on me", "MERGE  conflicts with main"} {
		if !strings.Contains(out, want) {
			t.Errorf("details lack %q:\n%s", want, out)
		}
	}
	m, _ = press(t, m, down)
	out = view(m)
	for _, want := range []string{"CHECKS  2 failed · 1 passed", "✖ test  CI", "✓ approved by sam", "cloned in /p/api"} {
		if !strings.Contains(out, want) {
			t.Errorf("details lack %q:\n%s", want, out)
		}
	}
}

func TestEnterOpensTheBrowser(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, Config{}, actions)
	m, cmd := press(t, m, enter)
	m = deliver(t, m, cmd)
	if len(actions.opened) != 1 || actions.opened[0] != "https://github.com/acme/web/pull/9" {
		t.Errorf("opened = %q", actions.opened)
	}
	if !strings.Contains(view(m), "Opened acme/web#9 in your browser") {
		t.Errorf("missing confirmation:\n%s", view(m))
	}
}

func TestCheckoutInTheLocalClone(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, Config{}, actions)
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("c"))
	m = deliver(t, m, cmd)
	if len(actions.clones) != 0 || !slices.Equal(actions.checkouts, []string{"/p/api acme/api#2"}) {
		t.Errorf("clones %q, checkouts %q", actions.clones, actions.checkouts)
	}
	if !strings.Contains(view(m), "Checked out fast-login in api") {
		t.Errorf("missing confirmation:\n%s", view(m))
	}
}

func TestCheckoutClonesARepositoryNotClonedYet(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, Config{}, actions)
	if !strings.Contains(view(m), "c clone ·") {
		t.Errorf("help should offer to clone:\n%s", view(m))
	}
	m, cmd := press(t, m, key("c"))
	if !strings.Contains(view(m), "Cloning acme/web into /p/web…") {
		t.Errorf("no progress line:\n%s", view(m))
	}
	m = deliver(t, m, cmd)
	if !slices.Equal(actions.clones, []string{"acme/web"}) || !slices.Equal(actions.checkouts, []string{"/p/web acme/web#9"}) {
		t.Errorf("clones %q, checkouts %q: want a clone, then a checkout in it", actions.clones, actions.checkouts)
	}
	out := view(m)
	for _, want := range []string{"Cloned acme/web into /p/web and checked out search", "cloned in /p/web", "c check out ·"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen lacks %q:\n%s", want, out)
		}
	}
}

func TestCloneFailureIsShown(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, Config{}, actions)
	m.cfg.Clone = func(context.Context, string) (string, error) { return "", pulls.ErrFolderTaken }
	m, cmd := press(t, m, key("c"))
	m = deliver(t, m, cmd)
	if len(actions.checkouts) != 0 || !strings.Contains(view(m), "could not clone acme/web into /p/web: already exists") {
		t.Errorf("checkouts %q:\n%s", actions.checkouts, view(m))
	}
}

func TestACloneIsKeptWhenItsCheckoutFails(t *testing.T) {
	m := loaded(t, Config{}, &fakeActions{})
	m.cfg.Checkout = func(context.Context, string, pulls.PR) error { return errors.New("no such branch") }
	m, cmd := press(t, m, key("c"))
	m = deliver(t, m, cmd)
	out := view(m)
	for _, want := range []string{"cloned acme/web into /p/web, but could not check out search: no such branch", "cloned in /p/web"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen lacks %q:\n%s", want, out)
		}
	}
}

func TestIDENeedsALocalClone(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, Config{IDEs: []ide.IDE{cursor}}, actions)
	m, _ = press(t, m, key("i"))
	if len(actions.ides) != 0 || !strings.Contains(view(m), "acme/web is not cloned in /p: press c to clone it") {
		t.Errorf("ides %q:\n%s", actions.ides, view(m))
	}
}

func TestCheckoutFailureIsShown(t *testing.T) {
	m := loaded(t, Config{}, &fakeActions{})
	m.cfg.Checkout = func(context.Context, string, pulls.PR) error { return pulls.ErrUncommitted }
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("c"))
	m = deliver(t, m, cmd)
	if !strings.Contains(view(m), "could not check out acme/api#2 in api: has uncommitted changes") {
		t.Errorf("the refusal should be explained:\n%s", view(m))
	}
}

func TestLogShowsEachFailedCheck(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, Config{}, actions)
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("l"))
	m = deliver(t, m, cmd)
	out := view(m)
	if m.state != stateViewingLog || !strings.Contains(out, "Log of test") || !strings.Contains(out, "failed check 1 of 2") || !strings.Contains(out, "test broke") {
		t.Fatalf("log view:\n%s", out)
	}
	m, cmd = press(t, m, tab)
	m = deliver(t, m, cmd)
	if !strings.Contains(view(m), "e2e broke") || strings.Join(actions.logs, ",") != "test,e2e" {
		t.Errorf("tab should read the next failed check: logs %q\n%s", actions.logs, view(m))
	}
	m, cmd = press(t, m, key("o"))
	deliver(t, m, cmd)
	if len(actions.opened) != 1 || actions.opened[0] != "https://github.com/acme/api/actions/runs/1/job/13" {
		t.Errorf("o should open the job page: %q", actions.opened)
	}
}

func TestLogWithoutFailedChecksExplains(t *testing.T) {
	m := loaded(t, Config{}, &fakeActions{})
	m, _ = press(t, m, key("l"))
	if m.state != stateListing || !strings.Contains(view(m), "acme/web#9 has no failed checks") {
		t.Errorf("state %v:\n%s", m.state, view(m))
	}
}

func TestLogErrorOffersTheBrowser(t *testing.T) {
	m := loaded(t, Config{}, &fakeActions{})
	m.cfg.FailedLog = func(context.Context, pulls.Check) ([]pulls.LogLine, error) { return nil, pulls.ErrLogExpired }
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("l"))
	m = deliver(t, m, cmd)
	if out := view(m); !strings.Contains(out, "Actions logs expire after 90 days") || !strings.Contains(out, "Press o to open the job") {
		t.Errorf("log error:\n%s", out)
	}
	m, _ = press(t, m, esc)
	if m.state != stateListing {
		t.Errorf("esc should go back to the list")
	}
}

func TestOpenInIDE(t *testing.T) {
	actions := &fakeActions{}
	cfg := Config{IDEs: []ide.IDE{cursor, zed}, ProjectIDEs: map[string]string{"/p/api": zed.AppPath}}
	m := loaded(t, cfg, actions)
	m, _ = press(t, m, down)
	m, _ = press(t, m, key("i"))
	if m.state != stateChoosingIDE || m.picker.Current() != zed {
		t.Fatalf("state %v, preselected %v", m.state, m.picker.Current())
	}
	if !strings.Contains(view(m), "Open api on fast-login with") {
		t.Errorf("the IDE box should name the branch:\n%s", view(m))
	}
	m, cmd := press(t, m, enter)
	m = deliver(t, m, cmd)
	if !slices.Equal(actions.checkouts, []string{"/p/api acme/api#2"}) || !slices.Equal(actions.ides, []string{"/p/api in Zed"}) {
		t.Errorf("checkouts %q, ides %q: want the branch checked out, then the IDE opened", actions.checkouts, actions.ides)
	}
	if !strings.Contains(view(m), "Checked out fast-login in api and opened it in Zed") {
		t.Errorf("missing confirmation:\n%s", view(m))
	}
}

func TestOpenInIDEOnTheBranchAlreadySkipsTheCheckout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "api")
	if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/fast-login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	actions := &fakeActions{}
	m := loaded(t, Config{IDEs: []ide.IDE{cursor}}, actions)
	m.clones = map[string]string{"acme/api": dir}
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("i"))
	m = deliver(t, m, cmd)
	if len(actions.checkouts) != 0 || len(actions.ides) != 1 || !strings.Contains(view(m), "Opened api in Cursor on fast-login") {
		t.Errorf("checkouts %q, ides %q:\n%s", actions.checkouts, actions.ides, view(m))
	}
}

func TestOpenInIDEOpensNothingWhenTheCheckoutIsRefused(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, Config{IDEs: []ide.IDE{cursor}}, actions)
	m.cfg.Checkout = func(context.Context, string, pulls.PR) error { return pulls.ErrUncommitted }
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("i"))
	m = deliver(t, m, cmd)
	if len(actions.ides) != 0 || !strings.Contains(view(m), "could not check out acme/api#2 in api: has uncommitted changes") {
		t.Errorf("ides %q:\n%s", actions.ides, view(m))
	}
}

func TestNarrowTerminalDropsTheSize(t *testing.T) {
	const narrowWidth = 60
	m := loaded(t, Config{}, &fakeActions{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: narrowWidth, Height: 30})
	out := view(next.(Model))
	row := ""
	for line := range strings.Lines(out) {
		if strings.Contains(line, "web#9 ") {
			row = line
		}
	}
	if row == "" || strings.Contains(row, "+0 −0") {
		t.Errorf("a narrow terminal has no room for the size column: %q", row)
	}
	for line := range strings.Lines(out) {
		if w := ansi.StringWidth(strings.TrimRight(line, "\n")); w > narrowWidth {
			t.Errorf("line of %d columns overflows %d: %q", w, narrowWidth, line)
		}
	}
}

func TestLoadErrorEndsTheScreen(t *testing.T) {
	next, _ := New(context.Background(), Config{}).Update(loadedMsg{err: pulls.ErrNotLoggedIn})
	if !errors.Is(next.(Model).Err(), pulls.ErrNotLoggedIn) {
		t.Errorf("Err() = %v", next.(Model).Err())
	}
}

func TestPlainRow(t *testing.T) {
	p := sampleDashboard().ToReview[0]
	got := strings.Join(PlainRow(p, "me", time.Now()), "|")
	if got != "acme/web#9|Add search  @kim|conflicts|–|1h 0m ago|https://github.com/acme/web/pull/9" {
		t.Errorf("PlainRow = %q", got)
	}
}
