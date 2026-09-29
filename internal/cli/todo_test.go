package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
)

func TestWriteTodosPrintsOneRowPerComment(t *testing.T) {
	now := time.Now()
	items := []todos.Item{
		{Project: "api", File: "a.ts", Line: 3, Marker: todos.Fixme, Note: "off by one", Author: "Kim", At: now.Add(-400 * 24 * time.Hour)},
		{Project: "web", File: "b.ts", Line: 1, Marker: todos.Todo, Uncommitted: true, Mine: true},
	}
	var out bytes.Buffer
	if err := writeTodos(&out, items, now); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "AGE") {
		t.Fatalf("want a header and two rows:\n%s", out.String())
	}
	for i, want := range []string{"1 year         FIXME   api      a.ts:3  Kim     off by one", "not committed  TODO    web      b.ts:1  you     no note"} {
		if lines[i+1] != want {
			t.Errorf("row %d = %q, want %q", i+1, lines[i+1], want)
		}
	}
}
