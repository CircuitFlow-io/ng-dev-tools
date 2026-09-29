package envfiles

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const secretValue = "sk_live_do-not-print"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		role Role
		ok   bool
	}{
		{".env", Local, true},
		{".env.local", Local, true},
		{".env.development.local", Local, true},
		{"app.env", Local, true},
		{".env.example", Example, true},
		{".env.local.sample", Example, true},
		{"app.env.template", Example, true},
		{".env.default", Defaults, true},
		{".xcode.env", 0, false},
		{".envrc", 0, false},
		{"nativewind-env.d.ts", 0, false},
	}
	for _, tt := range tests {
		role, ok := classify(tt.name)
		if role != tt.role || ok != tt.ok {
			t.Errorf("classify(%q) = %v, %v; want %v, %v", tt.name, role, ok, tt.role, tt.ok)
		}
	}
}

func TestParseKeys(t *testing.T) {
	content := "# Stripe\n# from the dashboard\nSTRIPE_KEY=" + secretValue + "\n\n" +
		"export DATABASE_URL = postgres://x\n" +
		"EMPTY=\nQUOTED_EMPTY=\"\"\nCOMMENT_ONLY= # set me\n" +
		"PRIVATE_KEY=\"-----BEGIN KEY-----\nNOT_A_KEY=inside the value\n-----END KEY-----\"\n" +
		"not a line\n" +
		"EMPTY=now set\n"
	keys, err := parseKeys(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	empty := map[string]bool{}
	for _, k := range keys {
		names = append(names, k.Name)
		empty[k.Name] = k.Empty
	}
	if want := []string{"STRIPE_KEY", "DATABASE_URL", "EMPTY", "QUOTED_EMPTY", "COMMENT_ONLY", "PRIVATE_KEY"}; !slices.Equal(names, want) {
		t.Fatalf("names = %q, want %q", names, want)
	}
	if empty["STRIPE_KEY"] || empty["PRIVATE_KEY"] || empty["EMPTY"] || !empty["QUOTED_EMPTY"] || !empty["COMMENT_ONLY"] {
		t.Errorf("empty = %v", empty)
	}
	if !slices.Equal(keys[0].Comments, []string{"# Stripe", "# from the dashboard"}) || keys[1].Comments != nil {
		t.Errorf("comments = %q, %q", keys[0].Comments, keys[1].Comments)
	}
	if strings.Contains(fmt.Sprintf("%+v", keys), secretValue) {
		t.Error("parsed keys hold a value")
	}
}

func TestScanProjectComparesKeys(t *testing.T) {
	root := t.TempDir()
	web := filepath.Join(root, "shop", "apps", "web")
	write(t, web, ".env.example", "# The API\nAPI_URL=https://example.com\nSTRIPE_KEY=\nSENTRY_DSN=\nFEATURE_FLAG=\n")
	write(t, web, ".env", "API_URL=http://localhost\nSTRIPE_KEY=\nOLD_KEY=1\n")
	write(t, web, ".env.local", "STRIPE_KEY="+secretValue+"\nSENTRY_DSN=\n")
	write(t, web, "src/client.ts", "const a = process.env.API_URL\nconst b = process.env['ANALYTICS_ID']\nif (process.env.NODE_ENV) {}\nimport.meta.env.VITE_HIDDEN\n")
	write(t, web, "node_modules/lib/.env", "IGNORED=1\n")
	write(t, web, "node_modules/lib/index.js", "process.env.FROM_A_DEPENDENCY\n")
	write(t, filepath.Join(root, "shop", "apps", "demo"), ".env.example", "A=\n")
	write(t, filepath.Join(root, "shop", "ios"), ".xcode.env", "export NODE_BINARY=node\n")
	write(t, filepath.Join(root, "shop", "scripts"), ".env", "LONELY=1\n")

	sets := ScanProject(context.Background(), macos.ExecRunner{}, root, filepath.Join(root, "shop"))
	Sort(sets)
	byName := map[string]Set{}
	for _, s := range sets {
		byName[s.Name] = s
	}
	if len(sets) != 3 {
		t.Fatalf("sets = %+v, want web, demo and scripts", sets)
	}

	web1 := byName["shop/apps/web"]
	if !slices.Equal(web1.Locals, []string{".env", ".env.local"}) || web1.ExampleKeys != 4 {
		t.Errorf("web = %+v", web1)
	}
	if !slices.Equal(web1.Missing, []string{"FEATURE_FLAG"}) {
		t.Errorf("missing = %q, want the key no file has", web1.Missing)
	}
	if want := []KeyInFile{{"SENTRY_DSN", ".env.local"}}; !slices.Equal(web1.Empty, want) {
		t.Errorf("empty = %v, want %v: STRIPE_KEY is set in .env.local", web1.Empty, want)
	}
	if want := []KeyInFile{{"OLD_KEY", ".env"}}; !slices.Equal(web1.Extra, want) {
		t.Errorf("extra = %v, want %v", web1.Extra, want)
	}
	if want := []CodeRef{{"ANALYTICS_ID", "src/client.ts", 2}, {"VITE_HIDDEN", "src/client.ts", 4}}; !slices.Equal(web1.InCode, want) {
		t.Errorf("in code = %v, want %v", web1.InCode, want)
	}
	if web1.Attention() != Missing {
		t.Errorf("web attention = %v", web1.Attention())
	}
	if demo := byName["shop/apps/demo"]; demo.Attention() != NotSetUp || demo.Missing != nil {
		t.Errorf("demo = %+v", demo)
	}
	if scripts := byName["shop/scripts"]; scripts.Attention() != NoExample || scripts.Example != "" {
		t.Errorf("scripts = %+v", scripts)
	}
	if strings.Contains(fmt.Sprintf("%+v", sets), secretValue) {
		t.Error("a set holds a value")
	}
}

func TestClosestExamplePairsLocalFiles(t *testing.T) {
	root := t.TempDir()
	lane := filepath.Join(root, "app", "fastlane")
	write(t, lane, ".env.example", "APPLE_ID=\n")
	write(t, lane, ".env.local.example", "PASSWORD=\n")
	write(t, lane, ".env.default", "APPLE_ID=team@example.com\n")
	write(t, lane, ".env.local", "PASSWORD=x\n")

	sets := ScanProject(context.Background(), macos.ExecRunner{}, root, filepath.Join(root, "app"))
	if len(sets) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	for _, s := range sets {
		switch s.Example {
		case ".env.example":
			if s.Locals != nil || !slices.Equal(s.Defaults, []string{".env.default"}) || s.Attention() != NotSetUp {
				t.Errorf(".env.example set = %+v", s)
			}
		case ".env.local.example":
			if !slices.Equal(s.Locals, []string{".env.local"}) || s.Attention() != Complete {
				t.Errorf(".env.local.example set = %+v", s)
			}
		}
	}
}

func TestScanProjectFindsCommittedFiles(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	dir := filepath.Join(root, "api")
	runGit(t, root, "init", "--quiet", "--initial-branch=main", "api")
	write(t, dir, ".env.example", "TOKEN=\n")
	write(t, dir, ".env", "TOKEN=x\n")
	write(t, dir, "old/.env.production", "TOKEN=y\n")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "--quiet", "-m", "add env")
	runGit(t, dir, "rm", "--quiet", "-r", "old")
	runGit(t, dir, "-c", "user.name=T", "-c", "user.email=t@example.com", "commit", "--quiet", "-m", "remove old")

	sets := ScanProject(context.Background(), macos.ExecRunner{}, root, dir)
	Sort(sets)
	if len(sets) != 2 {
		t.Fatalf("sets = %+v", sets)
	}
	current, old := sets[0], sets[1]
	if current.Name != "api" || !slices.Equal(current.Tracked, []string{".env"}) || current.Committed != nil {
		t.Errorf("api = %+v, want .env tracked and the example not flagged", current)
	}
	if old.Name != filepath.Join("api", "old") || len(old.Committed) != 1 || old.Committed[0].File != ".env.production" || old.Committed[0].Hash == "" {
		t.Errorf("old = %+v, want the deleted .env.production in history", old)
	}
	if current.Attention() != Exposed || old.Attention() != Exposed {
		t.Errorf("attention = %v, %v", current.Attention(), old.Attention())
	}
}

func TestScanProjectWithoutCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet", "fresh")
	write(t, filepath.Join(root, "fresh"), ".env", "A=1\n")
	sets := ScanProject(context.Background(), macos.ExecRunner{}, root, filepath.Join(root, "fresh"))
	if len(sets) != 1 || sets[0].Err != nil {
		t.Errorf("sets = %+v", sets)
	}
}

func TestAddMissing(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "web")
	write(t, dir, ".env.example", "API_URL=\n\n# Get it from the Stripe dashboard\nSTRIPE_KEY=sk_test_placeholder\nSENTRY_DSN=\n")
	write(t, dir, ".env", "API_URL="+secretValue)
	write(t, dir, ".env.local", "SENTRY_DSN=x\n")
	set := ScanProject(context.Background(), macos.ExecRunner{}, root, dir)[0]

	target, added, err := AddMissing(set)
	if err != nil || target != ".env" || added != 1 {
		t.Fatalf("AddMissing = %q, %d, %v", target, added, err)
	}
	want := "API_URL=" + secretValue + "\n\n# Get it from the Stripe dashboard\nSTRIPE_KEY=\n"
	if got := read(t, dir, ".env"); got != want {
		t.Errorf(".env = %q, want %q", got, want)
	}
	if _, added, _ := AddMissing(set); added != 0 {
		t.Errorf("a second AddMissing added %d keys", added)
	}

	notSetUp := Set{Dir: dir, Example: ".env.example"}
	if _, _, err := AddMissing(notSetUp); !errors.Is(err, ErrNoLocalFile) {
		t.Errorf("without a local file: %v", err)
	}
}

func TestPipesAreNeverOpened(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "api")
	write(t, dir, ".env.example", "TOKEN=\n")
	write(t, dir, ".env.local", "OTHER=1\n")
	if err := syscall.Mkfifo(filepath.Join(dir, ".env"), 0o600); err != nil {
		t.Skip("cannot make a named pipe:", err)
	}

	done := make(chan []Set)
	go func() { done <- ScanProject(context.Background(), macos.ExecRunner{}, root, dir) }()
	var sets []Set
	select {
	case sets = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the scan opened the pipe and is waiting for a writer")
	}
	s := sets[0]
	if !slices.Equal(s.Locals, []string{".env", ".env.local"}) || !slices.Equal(s.Unread, []string{".env"}) {
		t.Fatalf("set = %+v, want .env listed but unread", s)
	}
	if s.Missing != nil || s.Empty != nil || s.Attention() != Undocumented {
		t.Errorf("missing and empty are unknown with a pipe: %+v", s)
	}
	if target, ok := s.AddTarget(); !ok || target != ".env.local" {
		t.Errorf("AddTarget = %q, %v; want the regular file", target, ok)
	}
}

func TestSortPutsExposedFirst(t *testing.T) {
	sets := []Set{
		{Name: "b", Example: ".env.example", Locals: []string{".env"}},
		{Name: "c", Example: ".env.example"},
		{Name: "a", Example: ".env.example", Locals: []string{".env"}, Missing: []string{"X"}},
		{Name: "d", Locals: []string{".env"}, Tracked: []string{".env"}},
	}
	Sort(sets)
	var names []string
	for _, s := range sets {
		names = append(names, s.Name)
	}
	if !slices.Equal(names, []string{"d", "a", "c", "b"}) {
		t.Errorf("order = %q", names)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, dir, name string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
}
