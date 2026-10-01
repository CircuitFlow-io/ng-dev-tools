package todos

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

func TestParseMarker(t *testing.T) {
	tests := []struct {
		path, line string
		marker     Marker
		note       string
		ok         bool
	}{
		{"a.ts", "  // TODO: Calculate cost based on provider pricing", Todo, "Calculate cost based on provider pricing", true},
		{"a.ts", "    // @ts-expect-error FIXME", Fixme, "@ts-expect-error", true},
		{"a.tsx", "  {/* TODO: Load GPX */}", Todo, "Load GPX", true},
		{"a.go", "\t * HACK(sam) - works around the SDK", Hack, "works around the SDK", true},
		{"a.py", "x = 1  # FIXME off by one", Fixme, "off by one", true},
		{"a.sql", "-- TODO add an index", Todo, "add an index", true},
		{"README.md", "> **TODO**: Full GPX rendering", Todo, "Full GPX rendering", true},
		{"README.md", "  id: 'TODO',", "", "", false},
		{"a.ts", "const status = 'TODO'", "", "", false},
		{"a.go", `Example: "ngt todo --mine | grep FIXME",`, "", "", false},
		{"a.go", "// Package todos finds the TODO comments", "", "", false},
		{"a.ts", "// todo: lowercase is prose", "", "", false},
		{"a.ts", "// TODOS are not a marker", "", "", false},
		{"a_test.go", `{"a.ts", "  // TODO: in a test string"},`, "", "", false},
		{"a.ts", `fetch("https://x.dev") // TODO: retry`, Todo, "retry", true},
	}
	for _, tt := range tests {
		marker, note, ok := parseMarker(tt.path, tt.line)
		if marker != tt.marker || note != tt.note || ok != tt.ok {
			t.Errorf("parseMarker(%q, %q) = %q, %q, %v; want %q, %q, %v", tt.path, tt.line, marker, note, ok, tt.marker, tt.note, tt.ok)
		}
	}
}

func TestParseBlame(t *testing.T) {
	out := "e592be74c1bbbd008c83b9723e71b2f0b63afb5a 83 85 1\n" +
		"author Sam\nauthor-mail <sam@example.com>\nauthor-time 1767698152\nsummary Use batches\nfilename a.ts\n" +
		"\t    // @ts-expect-error FIXME\n" +
		"0000000000000000000000000000000000000000 3 3 1\n" +
		"author Not Committed Yet\nauthor-mail <not.committed.yet>\nauthor-time 1790000000\nsummary Version of a.ts from a.ts\nfilename a.ts\n" +
		"\t// TODO new\n"
	blames := parseBlame(out)
	if b := blames[85]; b.author != "Sam" || b.email != "sam@example.com" || b.subject != "Use batches" || b.at.Unix() != 1767698152 {
		t.Errorf("line 85 = %+v", b)
	}
	if blames[3].hash != uncommittedHash {
		t.Errorf("line 3 = %+v, want uncommitted", blames[3])
	}
}

func TestSortPutsOldestFirstAndUncommittedLast(t *testing.T) {
	now := time.Now()
	items := []Item{
		{File: "new", At: now},
		{File: "draft", Uncommitted: true},
		{File: "old", At: now.Add(-365 * 24 * time.Hour)},
	}
	Sort(items)
	var files []string
	for _, i := range items {
		files = append(files, i.File)
	}
	if !slices.Equal(files, []string{"old", "new", "draft"}) {
		t.Errorf("order = %q", files)
	}
}

func TestFindWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	dir := filepath.Join(root, "api")
	runGit(t, root, "init", "--quiet", "--initial-branch=main", "api")
	runGit(t, dir, "config", "user.email", "me@example.com")
	write(t, dir, "old.ts", "export const a = 1\n// FIXME from someone else\n")
	write(t, dir, "node_modules/lib/index.js", "// TODO in a dependency\n")
	write(t, dir, ".gitignore", "node_modules\n")
	runGit(t, dir, "add", ".")
	commitAs(t, dir, "Kim", "kim@example.com", "2024-01-01T10:00:00Z", "add old")
	write(t, dir, "mine.ts", "// TODO: mine\n")
	runGit(t, dir, "add", "mine.ts")
	commitAs(t, dir, "Me", "me@example.com", "2025-01-01T10:00:00Z", "add mine")
	write(t, dir, "mine.ts", "// TODO: mine\n// HACK not committed\n")
	write(t, dir, "draft.ts", "// TODO untracked file\n")

	items, err := Find(context.Background(), macos.ExecRunner{}, "api", dir)
	if err != nil {
		t.Fatal(err)
	}
	Sort(items)
	var got []string
	for _, i := range items {
		got = append(got, i.Location()+" "+string(i.Marker))
	}
	want := []string{"old.ts:2 FIXME", "mine.ts:1 TODO", "draft.ts:1 TODO", "mine.ts:2 HACK"}
	if !slices.Equal(got, want) {
		t.Fatalf("items = %q, want %q", got, want)
	}
	old, mine, untracked := items[0], items[1], items[2]
	if old.Author != "Kim" || old.Mine || old.Subject != "add old" || old.At.Year() != 2024 || old.CommitURL != "" {
		t.Errorf("old = %+v, want Kim's, not yours, and no GitHub link without a remote", old)
	}
	if !mine.Mine || mine.Uncommitted {
		t.Errorf("mine = %+v", mine)
	}
	if !untracked.Uncommitted || !untracked.Mine {
		t.Errorf("untracked = %+v, want uncommitted and yours", untracked)
	}
}

func TestFindWithoutMatches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet", "clean")
	write(t, filepath.Join(root, "clean"), "a.ts", "export {}\n")
	items, err := Find(context.Background(), macos.ExecRunner{}, "clean", filepath.Join(root, "clean"))
	if err != nil || items != nil {
		t.Errorf("Find = %v, %v; want nothing and no error", items, err)
	}
}

func TestRepoLinksPushedCommitsOnly(t *testing.T) {
	r := repo{me: identity{emails: []string{"me@example.com"}}, github: "acme/api", unpushed: map[string]bool{"bbb": true}}
	pushed := Item{Commit: "aaa", Email: "ME@example.com"}
	r.finish(&pushed)
	if pushed.CommitURL != "https://github.com/acme/api/commit/aaa" || !pushed.Mine {
		t.Errorf("pushed = %+v", pushed)
	}
	local := Item{Commit: "bbb"}
	r.finish(&local)
	if local.CommitURL != "" {
		t.Errorf("an unpushed commit got a link: %+v", local)
	}
}

func TestSurroundings(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a.ts", "1\n2\n3\n4\n5\n6\n")
	lines, err := Surroundings(Item{Dir: dir, File: "a.ts", Line: 2}, 2, 2)
	if err != nil || len(lines) != 4 || lines[0].Number != 1 || lines[3].Text != "4" {
		t.Errorf("Surroundings = %+v, %v", lines, err)
	}
}

func commitAs(t *testing.T, dir, name, email, date, message string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "commit", "--quiet", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+name, "GIT_AUTHOR_EMAIL="+email, "GIT_AUTHOR_DATE="+date,
		"GIT_COMMITTER_NAME="+name, "GIT_COMMITTER_EMAIL="+email, "GIT_COMMITTER_DATE="+date)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
}
