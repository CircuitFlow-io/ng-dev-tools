package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
)

func TestWriteEnvPrintsOneRowPerSet(t *testing.T) {
	sets := []envfiles.Set{
		{Name: "api", Example: ".env.example", Locals: []string{".env"}, Tracked: []string{".env"}},
		{Name: "web", Example: ".env.example", Locals: []string{".env", ".env.local"}, Missing: []string{"STRIPE_KEY", "SENTRY_DSN"}},
		{Name: "demo", Example: ".env.example"},
	}
	var out bytes.Buffer
	if err := writeEnv(&out, sets); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[0], "FOLDER") {
		t.Fatalf("want a header and three rows:\n%s", out.String())
	}
	for i, want := range []string{"tracked by git", ".env, .env.local  2", "no .env yet"} {
		if !strings.Contains(lines[i+1], want) {
			t.Errorf("row %d = %q, want %q", i+1, lines[i+1], want)
		}
	}
}
