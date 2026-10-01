package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
)

func sampleSessions(project string) []claudesessions.Session {
	now := time.Now()
	return []claudesessions.Session{
		{
			ID: "s1", Title: "Global sessions list", Dir: project, Branch: "feat/claude-sessions", Branches: []string{"main", "feat/claude-sessions"},
			FirstPrompt: "implement ngt claude sessions", LastPrompt: "open a PR", Prompts: 12,
			Started: now.Add(-4 * 24 * time.Hour), LastActive: now.Add(-3 * 24 * time.Hour),
			Models: []claudesessions.ModelUse{{ID: "claude-opus-5-5", Replies: 40}, {ID: "claude-haiku-4-5-20251001", Replies: 2}},
			PRs:    []string{"https://github.com/o/r/pull/9"},
			Turns:  []claudesessions.Turn{{Yours: true, Text: "implement ngt claude sessions"}, {Text: "The resume runs in the right folder."}},
		},
		{
			ID: "s2", Dir: "/gone/express-test", FirstPrompt: "create an express boilerplate", LastPrompt: "create an express boilerplate", Prompts: 1,
			LastActive: now.Add(-6 * 24 * time.Hour),
			Models:     []claudesessions.ModelUse{{ID: "claude-sonnet-5", Replies: 5}},
			Turns:      []claudesessions.Turn{{Yours: true, Text: "create an express boilerplate"}},
		},
	}
}

// loaded is the screen after reading the sample sessions, the first of which is in a project that
// exists in a temporary home.
func loaded(t *testing.T, cfg Config) Model {
	t.Helper()
	cfg.Home = t.TempDir()
	cfg.Root, cfg.Dir = filepath.Join(cfg.Home, "projects"), filepath.Join(cfg.Home, ".claude", "projects")
	project := filepath.Join(cfg.Root, "ng-dev-tools")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	next, _ := New(context.Background(), cfg).Update(foundMsg{sessions: sampleSessions(project)})
	return next.(Model)
}

func press(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, key := range keys {
		next, _ := m.Update(key)
		m = next.(Model)
	}
	return m
}

func typed(text string) []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, 0, len(text))
	for _, r := range text {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

var (
	enter     = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc       = tea.KeyPressMsg{Code: tea.KeyEscape}
	down      = tea.KeyPressMsg{Code: tea.KeyDown}
	backspace = tea.KeyPressMsg{Code: tea.KeyBackspace}
)

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

func TestListShowsEachSessionsFacts(t *testing.T) {
	screen := view(loaded(t, Config{}))

	for _, want := range []string{
		"2 sessions in 2 folders", "implement ngt claude sessions", "feat/claude-sessions", "12", "Opus 5.5", "3 days ago", "ng-dev-tools",
		"Global sessions list  s1", "~/projects/ng-dev-tools · on main, feat/claude-sessions", "Opus 5.5 (40 replies), Haiku 4.5 (2 replies)",
		"https://github.com/o/r/pull/9", "Last    open a PR",
	} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen is missing %q:\n%s", want, screen)
		}
	}
}

func TestTypingSearchesTheConversation(t *testing.T) {
	m := press(t, loaded(t, Config{}), typed("right folder")...)

	if len(m.table.results) != 1 || m.table.results[0].Session.ID != "s1" {
		t.Fatalf("results = %v", m.table.results)
	}
	screen := view(m)
	if !strings.Contains(screen, "1 of 2 sessions match") || !strings.Contains(screen, "Claude  The resume runs in the right folder.") {
		t.Errorf("screen:\n%s", screen)
	}

	m = press(t, m, typed("zz")...)
	if !strings.Contains(view(m), "No sessions match") {
		t.Errorf("screen:\n%s", view(m))
	}
	m = press(t, m, backspace, backspace)
	if m.query != "right folder" || len(m.table.results) != 1 {
		t.Errorf("after backspace query %q shows %d", m.query, len(m.table.results))
	}
}

func TestLettersSearchInsteadOfMoving(t *testing.T) {
	m := press(t, loaded(t, Config{}), typed("j")...)

	if m.query != "j" || m.table.cursor.Index != 0 {
		t.Errorf("query %q, cursor %d", m.query, m.table.cursor.Index)
	}
}

func TestEscClearsTheSearchThenQuits(t *testing.T) {
	m := press(t, loaded(t, Config{Query: "express"}), esc)
	if m.query != "" || len(m.table.results) != 2 || m.state != stateListing {
		t.Fatalf("query %q, %d results, state %v", m.query, len(m.table.results), m.state)
	}
	m = press(t, m, esc)
	if m.state != stateDone {
		t.Errorf("second esc did not quit")
	}
	if _, ok := m.Chosen(); ok {
		t.Error("quitting chose a session")
	}
}

func TestEnterChoosesTheSessionToResume(t *testing.T) {
	next, cmd := loaded(t, Config{}).Update(enter)
	m := next.(Model)

	s, ok := m.Chosen()
	if !ok || s.ID != "s1" || cmd == nil {
		t.Errorf("Chosen = %q, %v", s.ID, ok)
	}
}

func TestEnterRefusesASessionWhoseFolderIsGone(t *testing.T) {
	m := press(t, loaded(t, Config{}), down)
	if !strings.Contains(view(m), "no longer exists, so it cannot be resumed") {
		t.Errorf("details do not warn:\n%s", view(m))
	}

	m = press(t, m, enter)
	if _, ok := m.Chosen(); ok || m.state != stateListing {
		t.Error("chose a session whose folder is gone")
	}
	if !strings.Contains(view(m), "claude cannot resume there") {
		t.Errorf("no warning:\n%s", view(m))
	}
}

func TestNarrowTerminalDropsModelThenBranch(t *testing.T) {
	m := loaded(t, Config{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	screen := view(next.(Model))

	header := strings.Split(screen, "\n")[4]
	if strings.Contains(header, "MODEL") || strings.Contains(header, "BRANCH") || !strings.Contains(header, "FIRST PROMPT") {
		t.Errorf("header at 80 columns: %q", header)
	}
}

func TestReadErrorEndsTheScreen(t *testing.T) {
	failure := errors.New("permission denied")
	next, cmd := New(context.Background(), Config{}).Update(foundMsg{err: failure})

	if !errors.Is(next.(Model).Err(), failure) || cmd == nil {
		t.Errorf("Err = %v", next.(Model).Err())
	}
}

func TestNoSessionsEndsTheScreen(t *testing.T) {
	next, _ := New(context.Background(), Config{Dir: "/home/.claude/projects", Home: "/home"}).Update(foundMsg{})

	if err := next.(Model).Err(); err == nil || !strings.Contains(err.Error(), "~/.claude/projects") {
		t.Errorf("Err = %v", err)
	}
}

func TestProjectLabel(t *testing.T) {
	tests := map[string]string{
		"/home/projects/memorit/apps/native": "memorit/apps/native",
		"/home/projects":                     "~/projects",
		"/home":                              "~",
		"/home/projects-old/x":               "~/projects-old/x",
		"/tmp/x":                             "/tmp/x",
	}
	for dir, want := range tests {
		if got := projectLabel(dir, "/home", "/home/projects"); got != want {
			t.Errorf("projectLabel(%q) = %q, want %q", dir, got, want)
		}
	}
}

func TestOpenSessionsShowWhatClaudeIsDoing(t *testing.T) {
	now := time.Now()
	live := map[string]claudesessions.Live{
		"s1": {Activity: claudesessions.Waiting, WaitingFor: "permission", Since: now.Add(-3 * time.Minute), PID: 4044, Entrypoint: "cli"},
	}
	m := loaded(t, Config{})
	next, cmd := m.Update(liveMsg{live: live})
	m = next.(Model)
	screen := view(m)

	if !strings.Contains(strings.Split(screen, "\n")[4], "STATUS") {
		t.Errorf("header has no STATUS:\n%s", screen)
	}
	for _, want := range []string{"● waiting", "waiting on permission for 3m in a terminal (pid 4044)"} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen is missing %q:\n%s", want, screen)
		}
	}
	if cmd != nil {
		t.Error("asked for a refresh without a Live reader")
	}

	m = press(t, m, down)
	if strings.Contains(view(m), "(pid") {
		t.Errorf("a closed session has a status:\n%s", view(m))
	}
}

func TestLiveStatusRefreshesUntilTheScreenCloses(t *testing.T) {
	cfg := Config{Live: func(context.Context) map[string]claudesessions.Live { return nil }}
	m := loaded(t, cfg)

	next, cmd := m.Update(liveMsg{live: map[string]claudesessions.Live{"s1": {Activity: claudesessions.Idle, PID: 1}}})
	if cmd == nil || !strings.Contains(view(next.(Model)), "○ idle") {
		t.Fatalf("no refresh scheduled, or no idle status:\n%s", view(next.(Model)))
	}

	m = press(t, next.(Model), esc)
	if _, cmd := m.Update(liveMsg{}); cmd != nil {
		t.Error("kept refreshing after quitting")
	}
}

func TestDetailsShowTheTokensUsed(t *testing.T) {
	m := loaded(t, Config{})
	if strings.Contains(view(m), "Tokens") {
		t.Errorf("a session without usage shows tokens:\n%s", view(m))
	}

	s1 := m.table.results[0].Session
	s1.Usage = claudesessions.Usage{Input: 82, CacheWrite: 109_248, CacheRead: 4_874_452, Output: 31_118, Context: 148_841}
	m.table.results[0].Session = s1
	want := "Tokens  148k context · 82 in · 31k out · 4.9M cache read · 109k cache written"
	if !strings.Contains(view(m), want) {
		t.Errorf("screen is missing %q:\n%s", want, view(m))
	}

	next, _ := m.Update(liveMsg{usage: map[string]claudesessions.Usage{"s1": {Context: 150_000, Output: 32_000}}})
	if !strings.Contains(view(next.(Model)), "150k context") {
		t.Errorf("live usage not shown:\n%s", view(next.(Model)))
	}
}

func TestRefreshFollowsTheTranscriptsOfOpenSessions(t *testing.T) {
	const reply = `{"type":"assistant","message":{"id":"%s","model":"claude-opus-5-5","usage":{"input_tokens":1,"cache_creation_input_tokens":0,"cache_read_input_tokens":%d,"output_tokens":5}}}` + "\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "open.jsonl")
	writeFile(t, path, `{"type":"user","cwd":"/p","message":{"role":"user","content":"hi"}}`+"\n"+fmt.Sprintf(reply, "m1", 100))
	open, err := claudesessions.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	m := New(context.Background(), Config{})
	m.sessions = map[string]claudesessions.Session{"open": open}
	live := map[string]claudesessions.Live{"open": {Activity: claudesessions.Working}, "elsewhere": {Activity: claudesessions.Idle}}

	appendFile(t, path, fmt.Sprintf(reply, "m2", 200))
	usage := m.followUsage(live)

	if got := usage["open"]; got.Output != 10 || got.Context != 201 {
		t.Errorf("open session's usage = %+v", got)
	}
	if len(usage) != 1 {
		t.Errorf("followed sessions it does not know: %v", usage)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func appendFile(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}
