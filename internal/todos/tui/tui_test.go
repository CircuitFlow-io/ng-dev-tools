package tui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
)

var (
	zed    = ide.IDE{Name: "Zed", AppPath: "/Applications/Zed.app"}
	cursor = ide.IDE{Name: "Cursor", AppPath: "/Applications/Cursor.app"}
)

func sampleItems(dir string) []todos.Item {
	now := time.Now()
	return []todos.Item{
		{
			Project: "museum", Dir: dir, File: "api/admin.ts", Line: 3, Marker: todos.Fixme, Note: "@ts-expect-error",
			Author: "Kim", At: now.Add(-400 * 24 * time.Hour), Commit: "e592be74c1bbbd008c83b9723e71b2f0b63afb5a",
			Subject: "Use batches", CommitURL: "https://github.com/acme/museum/commit/e592be7",
		},
		{
			Project: "weather", Dir: "/p/weather", File: "src/Day.tsx", Line: 147, Marker: todos.Todo, Note: "Load GPX",
			Author: "Me", Mine: true, At: now.Add(-60 * 24 * time.Hour), Commit: "abc1234def",
		},
		{Project: "ngt", Dir: "/p/ngt", File: "main.go", Line: 1, Marker: todos.Hack, Uncommitted: true, Mine: true},
	}
}

// fakeActions records what the screen asks to open.
type fakeActions struct {
	mu     sync.Mutex
	opened []string
	urls   []string
	err    error
}

func (f *fakeActions) openAt(item todos.Item, editor ide.IDE) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, item.Location()+" in "+editor.Name)
	return f.err
}

func (f *fakeActions) openURL(url string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	return nil
}

func loaded(t *testing.T, ides []ide.IDE, actions *fakeActions) Model {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api", "admin.ts"), []byte("const a = 1\nfunction b() {\n  // @ts-expect-error FIXME\n  call()\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Root: "/p", Home: "/home", IDEs: ides, OpenAt: actions.openAt, OpenURL: actions.openURL}
	next, _ := New(context.Background(), cfg).Update(foundMsg{items: sampleItems(dir), errs: map[string]error{"broken": errors.New("x")}})
	return next.(Model)
}

func press(t *testing.T, m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

// deliver runs cmd and feeds its message back to m.
func deliver(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	next, _ := m.Update(cmd())
	return next.(Model)
}

func key(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(text[0]), Text: text}
}

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

func TestListShowsOldestFirstWithCode(t *testing.T) {
	m := loaded(t, []ide.IDE{zed}, &fakeActions{})
	out := view(m)
	for _, want := range []string{
		"3 comments in 3 repos · 2 yours · oldest 1 year", "1 repo unreadable: broken",
		"▸ 1 year", "not committed", "you",
		"Added by Kim 1 year ago in e592be7 · Use batches",
		"3 │   // @ts-expect-error FIXME", "4 │   call()",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("view lacks %q:\n%s", want, out)
		}
	}
	if strings.Index(out, "@ts-expect-error") > strings.Index(out, "Load GPX") {
		t.Errorf("the oldest should come first:\n%s", out)
	}
}

func TestMineOnly(t *testing.T) {
	m := loaded(t, []ide.IDE{zed}, &fakeActions{})
	m, _ = press(t, m, key("m"))
	out := view(m)
	if strings.Contains(out, "@ts-expect-error  ") || !strings.Contains(out, "2 of 3 are yours") || !strings.Contains(out, "m show all") {
		t.Errorf("m should show only your comments:\n%s", out)
	}
	m, _ = press(t, m, key("m"))
	if !strings.Contains(view(m), "3 comments") {
		t.Errorf("m again should show all:\n%s", view(m))
	}
}

func TestEnterOpensAtTheLine(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, []ide.IDE{zed}, actions)
	m, cmd := press(t, m, enter)
	m = deliver(t, m, cmd)
	if len(actions.opened) != 1 || actions.opened[0] != "api/admin.ts:3 in Zed" {
		t.Fatalf("opened = %v", actions.opened)
	}
	if !strings.Contains(view(m), "Opened api/admin.ts:3 in Zed") {
		t.Errorf("flash missing:\n%s", view(m))
	}
}

func TestEnterAsksWhichIDEWhenSeveral(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, []ide.IDE{zed, cursor}, actions)
	m, _ = press(t, m, enter)
	if m.state != stateChoosingIDE {
		t.Fatalf("state = %v, want the IDE box", m.state)
	}
	m, _ = press(t, m, tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.state != stateListing || len(actions.opened) != 0 {
		t.Errorf("esc should go back without opening: %v", actions.opened)
	}
}

func TestOpenCommit(t *testing.T) {
	actions := &fakeActions{}
	m := loaded(t, []ide.IDE{zed}, actions)
	m, cmd := press(t, m, key("o"))
	deliver(t, m, cmd)
	if len(actions.urls) != 1 || actions.urls[0] != "https://github.com/acme/museum/commit/e592be7" {
		t.Errorf("urls = %v", actions.urls)
	}

	m, _ = press(t, m, down)
	m, cmd = press(t, m, key("o"))
	if cmd != nil || !strings.Contains(view(m), "abc1234 is not on GitHub") {
		t.Errorf("an unpushed commit should explain:\n%s", view(m))
	}
	m, _ = press(t, m, down)
	m, _ = press(t, m, key("o"))
	if !strings.Contains(view(m), "This line is not committed yet") {
		t.Errorf("an uncommitted line should explain:\n%s", view(m))
	}
}

func TestNarrowTerminalFits(t *testing.T) {
	m := loaded(t, []ide.IDE{zed}, &fakeActions{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(Model)
	for line := range strings.Lines(view(m)) {
		if w := ansi.StringWidth(strings.TrimRight(line, "\n")); w > 80 {
			t.Errorf("line is %d wide: %q", w, line)
		}
	}
}

func TestEmptyList(t *testing.T) {
	cfg := Config{Root: "/p", Home: "/home"}
	next, _ := New(context.Background(), cfg).Update(foundMsg{})
	if out := view(next.(Model)); !strings.Contains(out, "Nothing to do") {
		t.Errorf("an empty list should say so:\n%s", out)
	}
}
