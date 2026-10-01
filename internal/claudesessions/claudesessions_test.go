package claudesessions

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// transcript is a session with the kinds of lines Claude Code writes, in file order.
const transcript = `{"type":"queue-operation","operation":"enqueue","content":"hi"}
{"type":"user","isMeta":true,"cwd":"/p/old-name","gitBranch":"HEAD","timestamp":"2026-09-01T10:00:00Z","message":{"role":"user","content":"<local-command-caveat>Caveat</local-command-caveat>"}}
{"type":"user","cwd":"/p/old-name","gitBranch":"HEAD","timestamp":"2026-09-01T10:00:01Z","message":{"role":"user","content":"<command-name>/model</command-name>"}}
{"type":"user","cwd":"/p/old-name","gitBranch":"main","timestamp":"2026-09-01T10:00:02Z","origin":{"kind":"human"},"message":{"role":"user","content":"Upgrade   the Expo\nSDK please"}}
{"type":"assistant","cwd":"/p/old-name","gitBranch":"main","timestamp":"2026-09-01T10:00:05Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"thinking","thinking":"secret"},{"type":"text","text":"Reading the changelog."}]}}
{"type":"assistant","cwd":"/p/old-name","gitBranch":"main","timestamp":"2026-09-01T10:00:06Z","message":{"id":"m1","model":"claude-opus-5-5","content":[{"type":"tool_use","name":"Bash","input":{"command":"ls"}}]}}
{"type":"user","cwd":"/p/old-name","gitBranch":"main","timestamp":"2026-09-01T10:00:07Z","toolUseResult":{"stdout":"x"},"message":{"role":"user","content":[{"type":"tool_result","content":"x"}]}}
{"type":"assistant","isSidechain":true,"cwd":"/p/old-name","timestamp":"2026-09-01T10:00:08Z","message":{"id":"m2","model":"claude-haiku-4-5-20251001","content":[{"type":"text","text":"subagent"}]}}
{"type":"user","cwd":"/p/old-name","gitBranch":"main","timestamp":"2026-09-01T10:00:09Z","origin":{"kind":"task-notification"},"message":{"role":"user","content":"<task-notification>done</task-notification>"}}
{"type":"user","isCompactSummary":true,"cwd":"/p/old-name","timestamp":"2026-09-01T10:00:10Z","message":{"role":"user","content":"This session is being continued"}}
{"type":"user","cwd":"/p/old-name","timestamp":"2026-09-01T10:00:11Z","message":{"role":"user","content":[{"type":"text","text":"[Request interrupted by user]"}]}}
{"type":"ai-title","aiTitle":"Expo upgrade"}
{"type":"custom-title","customTitle":"expo-57"}
{"type":"relocated","relocatedCwd":"/p/new-name"}
{"type":"assistant","cwd":"/p/new-name","gitBranch":"feat/expo","timestamp":"2026-09-02T08:00:00Z","message":{"id":"m3","model":"claude-sonnet-5","content":[{"type":"text","text":"Done."}]}}
{"type":"assistant","cwd":"/p/new-name","gitBranch":"feat/expo","timestamp":"2026-09-02T08:00:01Z","message":{"id":"m4","model":"<synthetic>","content":[{"type":"text","text":"API Error"}]}}
{"type":"pr-link","prUrl":"https://github.com/o/r/pull/7"}
{"type":"pr-link","prUrl":"https://github.com/o/r/pull/7"}
{"type":"user","cwd":"/p/new-name","gitBranch":"feat/expo","timestamp":"2026-09-02T08:01:00Z","message":{"role":"user","content":[{"type":"text","text":"[Image #1] now open a PR"},{"type":"image","source":{"data":"AAAA"}}]}}
{"type":"assistant","cwd":"/p/new-name","gitBranch":"feat/exp`

func writeTranscript(t *testing.T, dir, storage, id, content string) string {
	t.Helper()
	folder := filepath.Join(dir, storage)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, id+transcriptExt)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadGathersTheSession(t *testing.T) {
	path := writeTranscript(t, t.TempDir(), StorageName("/p/new-name"), "abc", transcript)
	s, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}

	if s.ID != "abc" || s.Dir != "/p/new-name" || s.Title != "expo-57" {
		t.Errorf("ID %q, Dir %q, Title %q", s.ID, s.Dir, s.Title)
	}
	if s.Prompts != 2 || s.FirstPrompt != "Upgrade the Expo SDK please" || s.LastPrompt != "[Image #1] now open a PR" {
		t.Errorf("%d prompts, first %q, last %q", s.Prompts, s.FirstPrompt, s.LastPrompt)
	}
	if s.Branch != "feat/expo" || !slices.Equal(s.Branches, []string{"main", "feat/expo"}) {
		t.Errorf("branch %q, branches %v", s.Branch, s.Branches)
	}
	wantModels := []ModelUse{{"claude-opus-5-5", 1}, {"claude-sonnet-5", 1}}
	if !slices.Equal(s.Models, wantModels) {
		t.Errorf("models = %v, want %v", s.Models, wantModels)
	}
	if !slices.Equal(s.PRs, []string{"https://github.com/o/r/pull/7"}) {
		t.Errorf("PRs = %v", s.PRs)
	}
	wantStart := time.Date(2026, 9, 1, 10, 0, 2, 0, time.UTC)
	wantLast := time.Date(2026, 9, 2, 8, 1, 0, 0, time.UTC)
	if !s.Started.Equal(wantStart) || !s.LastActive.Equal(wantLast) {
		t.Errorf("started %v, last active %v", s.Started, s.LastActive)
	}
	wantTurns := []Turn{
		{Yours: true, Text: "Upgrade the Expo SDK please"},
		{Text: "Reading the changelog."},
		{Text: "Done."},
		{Text: "API Error"},
		{Yours: true, Text: "[Image #1] now open a PR"},
	}
	if !slices.Equal(s.Turns, wantTurns) {
		t.Errorf("turns = %v", s.Turns)
	}
}

func TestResumeDirIsTheOneTheStorageFolderIsNamedAfter(t *testing.T) {
	cwds := []string{"/p/app", "/p/app/ios", "/p/app.v2"}
	tests := []struct {
		storage, relocated, want string
	}{
		{StorageName("/p/app/ios"), "", "/p/app/ios"},
		{StorageName("/p/app"), "", "/p/app"},
		{StorageName("/p/moved"), "/p/moved", "/p/moved"},
		{"-unknown", "", "/p/app"},
		{"-unknown", "/p/moved", "/p/moved"},
	}
	for _, tt := range tests {
		if got := resumeDir(tt.storage, tt.relocated, cwds); got != tt.want {
			t.Errorf("resumeDir(%q, %q) = %q, want %q", tt.storage, tt.relocated, got, tt.want)
		}
	}
}

func TestStorageName(t *testing.T) {
	if got := StorageName("/Users/me/my_app.v2/é"); got != "-Users-me-my-app-v2--" {
		t.Errorf("StorageName = %q", got)
	}
}

func TestIsMachineText(t *testing.T) {
	tests := map[string]bool{
		"<command-name>/clear</command-name>":             true,
		"<local-command-stdout>ok</local-command-stdout>": true,
		"<bash-input>ls</bash-input>":                     true,
		"[Request interrupted by user for tool use]":      true,
		"<div> is not rendering":                          false,
		"<Button> has the wrong color":                    false,
		"why is a < b here":                               false,
		"[Image #1] too much padding":                     false,
	}
	for text, want := range tests {
		if got := isMachineText(text); got != want {
			t.Errorf("isMachineText(%q) = %v, want %v", text, got, want)
		}
	}
}

func TestFindAllSkipsEmptySessionsAndSubagents(t *testing.T) {
	dir := t.TempDir()
	older := strings.ReplaceAll(transcript, "2026-09-02", "2026-08-02")
	writeTranscript(t, dir, "-p-new-name", "older", older)
	writeTranscript(t, dir, "-p-new-name", "newer", transcript)
	writeTranscript(t, dir, "-p-new-name", "empty", `{"type":"user","isMeta":true,"cwd":"/p","message":{"content":"<local-command-caveat>x</local-command-caveat>"}}`)
	writeTranscript(t, dir, filepath.Join("-p-new-name", "newer", "subagents"), "agent-1", transcript)

	sessions, errs, err := FindAll(context.Background(), dir)
	if err != nil || len(errs) != 0 {
		t.Fatalf("err %v, errs %v", err, errs)
	}
	var ids []string
	for _, s := range sessions {
		ids = append(ids, s.ID)
	}
	if !slices.Equal(ids, []string{"newer", "older"}) {
		t.Errorf("ids = %v, want the two sessions with prompts, newest first", ids)
	}
}

func TestFindAllWithoutSessionsFolder(t *testing.T) {
	_, _, err := FindAll(context.Background(), filepath.Join(t.TempDir(), "missing"))
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("err = %v, want not exist", err)
	}
}

func TestDir(t *testing.T) {
	env := map[string]string{}
	getenv := func(k string) string { return env[k] }
	if got := Dir("/home", getenv); got != "/home/.claude/projects" {
		t.Errorf("Dir = %q", got)
	}
	env[configDirVariable] = "/cfg"
	if got := Dir("/home", getenv); got != "/cfg/projects" {
		t.Errorf("Dir with %s = %q", configDirVariable, got)
	}
}

func TestModelName(t *testing.T) {
	tests := map[string]string{
		"claude-opus-5-5":            "Opus 5.5",
		"claude-sonnet-5":            "Sonnet 5",
		"claude-haiku-4-5-20251001":  "Haiku 4.5",
		"claude-opus-4-8":            "Opus 4.8",
		"claude-fable-5-1":           "Fable 5.1",
		"claude-3-5-sonnet-20241022": "Sonnet 3.5",
		"claude-opus-5-5[1m]":        "Opus 5.5 [1m]",
		"gpt-9":                      "gpt-9",
		"":                           "",
	}
	for id, want := range tests {
		if got := ModelName(id); got != want {
			t.Errorf("ModelName(%q) = %q, want %q", id, got, want)
		}
	}
}

func searchSessions() []Session {
	return []Session{
		{ID: "s1", Title: "Expo upgrade", Dir: "/p/memorit", Branches: []string{"upgrade-expo"}, Models: []ModelUse{{"claude-opus-5", 3}}, Turns: []Turn{
			{Yours: true, Text: "The exported types break after the Expo SDK upgrade"},
			{Text: "Expo SDK 57 renames the config plugin."},
		}},
		{ID: "s2", Title: "Telegram bot", Dir: "/p/trip", Branches: []string{"main"}, Models: []ModelUse{{"claude-sonnet-5", 1}}, Turns: []Turn{
			{Yours: true, Text: "can i message someone using a Telegram bot?"},
		}},
	}
}

func TestSearchNeedsEveryWord(t *testing.T) {
	x := NewIndex(searchSessions())
	tests := map[string][]string{
		"":              {"s1", "s2"},
		"EXPO sdk":      {"s1"},
		"telegram":      {"s2"},
		"telegram expo": nil,
		"trip main":     {"s2"},
		"sonnet 5":      {"s2"},
		"s1":            {"s1"},
	}
	for query, want := range tests {
		var got []string
		for _, r := range x.Search(query) {
			got = append(got, r.Session.ID)
		}
		if !slices.Equal(got, want) {
			t.Errorf("Search(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestSnippetPrefersThePhraseThenWholeWords(t *testing.T) {
	x := NewIndex(searchSessions())
	tests := []struct {
		query string
		want  Snippet
	}{
		{"expo sdk", Snippet{Yours: true, Before: "The exported types break after the ", Match: "Expo SDK", After: " upgrade"}},
		{"expo", Snippet{Yours: true, Before: "The exported types break after the ", Match: "Expo", After: " SDK upgrade"}},
		{"renames", Snippet{Before: "Expo SDK 57 ", Match: "renames", After: " the config plugin."}},
		{"export", Snippet{Yours: true, Before: "The ", Match: "export", After: "ed types break after the Expo SDK upgrade"}},
		{"memorit", Snippet{}},
	}
	for _, tt := range tests {
		results := x.Search(tt.query)
		if len(results) == 0 {
			t.Errorf("Search(%q) found nothing", tt.query)
			continue
		}
		if got := results[0].Snippet; got != tt.want {
			t.Errorf("Search(%q) snippet = %+v, want %+v", tt.query, got, tt.want)
		}
	}
}

func TestSnippetLeadIsCutAtAWord(t *testing.T) {
	long := strings.Repeat("word ", 20) + "needle here"
	x := NewIndex([]Session{{ID: "s", Turns: []Turn{{Text: long}}}})
	got := x.Search("needle")[0].Snippet
	if !strings.HasPrefix(got.Before, ellipsis+"word") || !strings.HasSuffix(got.Before, "word ") {
		t.Errorf("Before = %q", got.Before)
	}
}

func TestWithPWDReplacesTheOldValue(t *testing.T) {
	env := []string{"HOME=/h", "PWD=/old", "PATH=/bin"}
	got := withPWD(env, "/new")
	if !slices.Equal(got, []string{"HOME=/h", "PATH=/bin", "PWD=/new"}) || env[1] != "PWD=/old" {
		t.Errorf("withPWD = %v, env = %v", got, env)
	}
}

func TestReadSkipsLinesTooBigToHold(t *testing.T) {
	huge := `{"type":"user","toolUseResult":{"stdout":"` + strings.Repeat("x", maxRecordBytes) + `"}}`
	content := strings.Join([]string{
		`{"type":"user","cwd":"/p/api","timestamp":"2026-09-01T10:00:00Z","message":{"role":"user","content":"before"}}`,
		huge,
		`{"type":"user","cwd":"/p/api","timestamp":"2026-09-01T10:00:02Z","message":{"role":"user","content":"after"}}`,
	}, "\n")
	s, err := Read(writeTranscript(t, t.TempDir(), StorageName("/p/api"), "big", content))
	if err != nil {
		t.Fatal(err)
	}
	if s.Prompts != 2 || s.FirstPrompt != "before" || s.LastPrompt != "after" {
		t.Errorf("prompts %d, first %q, last %q: want the lines around the huge one", s.Prompts, s.FirstPrompt, s.LastPrompt)
	}
}

func TestAPastedPromptIsShortenedForDisplayButSearchedWhole(t *testing.T) {
	pasted := "find the bug " + strings.Repeat("é log line ", maxPreviewBytes) + "needle"
	line := `{"type":"user","cwd":"/p/api","timestamp":"2026-09-01T10:00:00Z","message":{"role":"user","content":"` + pasted + `"}}`
	s, err := Read(writeTranscript(t, t.TempDir(), StorageName("/p/api"), "paste", line))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.FirstPrompt) > maxPreviewBytes || !strings.HasPrefix(s.FirstPrompt, "find the bug é") || !utf8.ValidString(s.FirstPrompt) {
		t.Errorf("first prompt is %d bytes: %.40q", len(s.FirstPrompt), s.FirstPrompt)
	}
	if results := NewIndex([]Session{s}).Search("needle"); len(results) != 1 {
		t.Error("the end of the pasted prompt should still be searchable")
	}
}
