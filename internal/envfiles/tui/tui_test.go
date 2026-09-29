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

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
)

func sampleSets() []envfiles.Set {
	return []envfiles.Set{
		{Name: "weather", Dir: "/p/weather", Example: ".env.example", ExampleKeys: 4, Locals: []string{".env"}},
		{
			Name: "shop/apps/web", Dir: "/p/shop/apps/web", Example: ".env.example", ExampleKeys: 12,
			Locals: []string{".env", ".env.local"}, Missing: []string{"STRIPE_KEY", "SENTRY_DSN"},
			Empty:  []envfiles.KeyInFile{{Key: "API_URL", File: ".env.local"}},
			Extra:  []envfiles.KeyInFile{{Key: "OLD_FLAG", File: ".env"}},
			InCode: []envfiles.CodeRef{{Key: "ANALYTICS_ID", File: "src/client.ts", Line: 12}},
		},
		{
			Name: "api", Dir: "/p/api", Example: ".env.example", ExampleKeys: 3, Locals: []string{".env"},
			Committed: []envfiles.Commit{{File: ".env", Hash: "bf95cc2", At: time.Now().Add(-30 * 24 * time.Hour)}},
		},
		{Name: "demo", Dir: "/p/demo", Example: ".env.example", ExampleKeys: 2},
		{Name: "lane", Dir: "/p/lane", Example: "app.env.example", ExampleKeys: 2},
		{Name: "vault", Dir: "/p/vault", Example: ".env.example", ExampleKeys: 5, Locals: []string{".env"}, Unread: []string{".env"}},
	}
}

// fakeAdder records the sets the screen asks to add keys to.
type fakeAdder struct {
	mu    sync.Mutex
	added []string
	err   error
}

func (f *fakeAdder) add(s envfiles.Set) (string, int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.added = append(f.added, s.Name)
	return ".env", len(s.Missing), f.err
}

func loaded(t *testing.T, adder *fakeAdder) Model {
	t.Helper()
	cfg := Config{
		Root: "/p", Home: "/home",
		Scan:       func(context.Context) ([]envfiles.Set, error) { return sampleSets(), nil },
		AddMissing: adder.add,
	}
	next, _ := New(context.Background(), cfg).Update(scannedMsg{sets: sampleSets()})
	return next.(Model)
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
	case scannedMsg, addedMsg:
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func key(text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(text[0]), Text: text}
}

var down = tea.KeyPressMsg{Code: tea.KeyDown}

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
	m := loaded(t, &fakeAdder{})
	out := view(m)
	order := []string{"▸ ✖ api", "● shop/apps/web", "◦ demo", "◦ vault", "✓ weather"}
	last := -1
	for _, want := range order {
		i := strings.Index(out, want)
		if i < 0 || i < last {
			t.Fatalf("%q is out of order in\n%s", want, out)
		}
		last = i
	}
	if !strings.Contains(out, "6 folders in /p") || !strings.Contains(out, "2 need attention") {
		t.Errorf("title should count folders and the ones needing attention:\n%s", out)
	}
	for name, want := range map[string]string{
		"shop/apps/web": ".env, .env.local",
		"api":           "in git history",
		"demo":          "no .env yet",
		"lane":          "(app.env)",
		"vault":         ".env (pipe)",
	} {
		if row := rowOf(t, m, name); !strings.Contains(row, want) {
			t.Errorf("%s row %q lacks %q", name, row, want)
		}
	}
}

func TestDetailsShowKeyNamesAndHistory(t *testing.T) {
	m := loaded(t, &fakeAdder{})
	out := view(m)
	if !strings.Contains(out, ".env is in git history: added in bf95cc2 1 month ago") {
		t.Errorf("details should warn about the committed .env first:\n%s", out)
	}

	m, _ = press(t, m, down)
	out = view(m)
	for _, want := range []string{"MISSING  2", "STRIPE_KEY", "EMPTY  1", "API_URL  .env.local", "NOT IN EXAMPLE  1", "OLD_FLAG", "USED IN CODE  1", "ANALYTICS_ID  src/client.ts:12"} {
		if !strings.Contains(out, want) {
			t.Errorf("details lack %q:\n%s", want, out)
		}
	}
}

func TestPipesAreNotCompared(t *testing.T) {
	m := loaded(t, &fakeAdder{})
	for range 4 {
		m, _ = press(t, m, down)
	}
	if row := rowOf(t, m, "vault"); strings.Count(row, "?") != 2 {
		t.Errorf("missing and empty should be unknown: %q", row)
	}
	if !strings.Contains(view(m), ".env is a pipe") {
		t.Errorf("details should say the pipe is not read:\n%s", view(m))
	}
}

func TestAddMissingAsksFirst(t *testing.T) {
	adder := &fakeAdder{}
	m := loaded(t, adder)
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("a"))
	if cmd != nil || !strings.Contains(view(m), "Add 2 missing keys to shop/apps/web/.env with empty values?") {
		t.Fatalf("a should ask first:\n%s", view(m))
	}

	m, _ = press(t, m, key("n"))
	if len(adder.added) != 0 || strings.Contains(view(m), "missing keys to") {
		t.Fatalf("n should cancel: added %v\n%s", adder.added, view(m))
	}

	m, _ = press(t, m, key("a"))
	m, cmd = press(t, m, key("y"))
	m = deliver(t, m, cmd)
	if len(adder.added) != 1 || adder.added[0] != "shop/apps/web" {
		t.Errorf("added = %v", adder.added)
	}
	if !strings.Contains(view(m), "Added 2 keys to shop/apps/web/.env: fill in their values") {
		t.Errorf("the flash should confirm:\n%s", view(m))
	}
}

func TestAddMissingExplainsWhenThereIsNothingToAdd(t *testing.T) {
	adder := &fakeAdder{}
	m := loaded(t, adder)
	m, _ = press(t, m, down)
	m, _ = press(t, m, down)
	m, cmd := press(t, m, key("a"))
	if cmd != nil || len(adder.added) != 0 || !strings.Contains(view(m), "Copy .env.example to .env first") {
		t.Errorf("a on a folder without a local file should explain:\n%s", view(m))
	}
}

func TestAddMissingReportsFailure(t *testing.T) {
	m := loaded(t, &fakeAdder{err: errors.New("permission denied")})
	m, _ = press(t, m, down)
	m, _ = press(t, m, key("a"))
	m, cmd := press(t, m, key("y"))
	m = deliver(t, m, cmd)
	if !strings.Contains(view(m), "Could not add the keys: permission denied") {
		t.Errorf("the failure should show:\n%s", view(m))
	}
}

func TestNarrowTerminalDropsNotes(t *testing.T) {
	m := loaded(t, &fakeAdder{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 72, Height: 30})
	m = next.(Model)
	row := rowOf(t, m, "api")
	if strings.Contains(view(m), "NOTES") || strings.Contains(row, "history") {
		t.Errorf("notes should give way at 72 columns:\n%s", view(m))
	}
	for line := range strings.Lines(view(m)) {
		if w := ansi.StringWidth(strings.TrimRight(line, "\n")); w > 72 {
			t.Errorf("line is %d wide: %q", w, line)
		}
	}
}

func TestQuit(t *testing.T) {
	m := loaded(t, &fakeAdder{})
	_, cmd := press(t, m, key("q"))
	if cmd == nil {
		t.Fatal("q should quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Error("q should quit")
	}
}
