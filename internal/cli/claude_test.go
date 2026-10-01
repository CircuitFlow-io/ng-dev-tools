package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
)

func TestWriteClaudeSessionsPrintsOneRowPerSession(t *testing.T) {
	now := time.Now()
	results := []claudesessions.Result{
		{Session: claudesessions.Session{
			ID: "abc", Dir: "/home/projects/memorit", Branch: "main", Prompts: 12, LastActive: now.Add(-3 * 24 * time.Hour),
			Models: []claudesessions.ModelUse{{ID: "claude-opus-5-5", Replies: 3}}, FirstPrompt: "upgrade expo",
		}},
		{Session: claudesessions.Session{ID: "def", Dir: "/tmp/x", Prompts: 1, LastActive: now.Add(-time.Hour), FirstPrompt: strings.Repeat("a", 100)}},
	}
	live := map[string]claudesessions.Live{"abc": {Activity: claudesessions.Working}}
	var out bytes.Buffer
	if err := writeClaudeSessions(&out, results, live, now, "/home"); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "ACTIVE") {
		t.Fatalf("want a header and two rows:\n%s", out.String())
	}
	if want := "3 days ago  working  ~/projects/memorit  main    12       Opus 5.5  abc      upgrade expo"; lines[1] != want {
		t.Errorf("row 1 = %q, want %q", lines[1], want)
	}
	if !strings.Contains(lines[2], "closed   /tmp/x") || !strings.HasSuffix(lines[2], strings.Repeat("a", plainPromptWidth-1)+"…") {
		t.Errorf("row 2 = %q", lines[2])
	}
}
