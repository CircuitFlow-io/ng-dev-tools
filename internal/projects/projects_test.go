package projects

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
)

var base = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// write creates root/path with its parent folders and sets its modification time.
func write(t *testing.T, root, path string, modified time.Time) {
	t.Helper()
	full := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(full, modified, modified); err != nil {
		t.Fatal(err)
	}
}

// ageDirs sets every folder under root to long ago, so only file times decide activity.
func ageDirs(t *testing.T, root string) {
	t.Helper()
	old := base.AddDate(-1, 0, 0)
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		return os.Chtimes(path, old, old)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func names(found []Project) []string {
	out := make([]string, 0, len(found))
	for _, p := range found {
		out = append(out, p.Name)
	}
	return out
}

func scan(t *testing.T, root string, opened map[string]time.Time) []Project {
	t.Helper()
	found, err := Scan(context.Background(), root, opened)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

func TestScanFindsProjectsAndExpandsGroupingFolders(t *testing.T) {
	root := t.TempDir()
	write(t, root, "api/go.mod", base)
	write(t, root, "repo/.git/HEAD", base)
	write(t, root, "clients/web/package.json", base)
	write(t, root, "clients/mobile/app.json", base)
	write(t, root, "clients/placeholder/.idea/workspace.xml", base)
	write(t, root, "ios-only/App.xcodeproj/project.pbxproj", base)
	write(t, root, "empty/.DS_Store", base)
	write(t, root, ".hidden/package.json", base)

	got := names(scan(t, root, nil))
	slices.Sort(got)

	want := []string{"api", "clients/mobile", "clients/web", "ios-only", "repo"}
	if !slices.Equal(got, want) {
		t.Errorf("projects = %v, want %v", got, want)
	}
}

func TestScanSortsByLatestOpenOrChange(t *testing.T) {
	root := t.TempDir()
	write(t, root, "recent/main.go", base)
	write(t, root, "older/main.go", base.Add(-48*time.Hour))
	write(t, root, "opened/main.go", base.Add(-72*time.Hour))
	ageDirs(t, root)
	opened := map[string]time.Time{filepath.Join(root, "opened"): base.Add(time.Hour)}

	found := scan(t, root, opened)

	if got, want := names(found), []string{"opened", "recent", "older"}; !slices.Equal(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
	if found[0].ActivityVerb("opened") != "opened" || found[1].ActivityVerb("opened") != "changed" {
		t.Errorf("verbs = %q, %q", found[0].ActivityVerb("opened"), found[1].ActivityVerb("opened"))
	}
}

func TestLastChangedIgnoresGeneratedFolders(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "src/app.ts", base)
	write(t, dir, "node_modules/react/index.js", base.Add(24*time.Hour))
	write(t, dir, "dist/app.js", base.Add(24*time.Hour))
	write(t, dir, ".expo/cache", base.Add(24*time.Hour))
	ageDirs(t, dir)

	if got := lastChanged(context.Background(), dir); !got.Equal(base) {
		t.Errorf("lastChanged = %v, want %v", got, base)
	}
}

func TestLastChangedCountsCommits(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "main.go", base)
	write(t, dir, ".git/HEAD", base)
	write(t, dir, ".git/logs/HEAD", base.Add(time.Hour))
	ageDirs(t, dir)

	if got := lastChanged(context.Background(), dir); !got.Equal(base.Add(time.Hour)) {
		t.Errorf("lastChanged = %v, want the commit time", got)
	}
}

func TestBranch(t *testing.T) {
	tests := []struct {
		name, head, want string
	}{
		{"branch", "ref: refs/heads/feat/login\n", "feat/login"},
		{"detached", "3f9a1c2d4e5b6a7980\n", "3f9a1c2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.MkdirAll(filepath.Join(dir, ".git"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte(tt.head), 0o644); err != nil {
				t.Fatal(err)
			}
			if got := Branch(dir); got != tt.want {
				t.Errorf("Branch = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBranchFollowsWorktreeGitFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "main-repo", "worktrees", "wt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main-repo", "worktrees", "wt", "HEAD"), []byte("ref: refs/heads/fix\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	worktree := filepath.Join(dir, "wt")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, ".git"), []byte("gitdir: ../main-repo/worktrees/wt\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := Branch(worktree); got != "fix" {
		t.Errorf("Branch = %q, want fix", got)
	}
	if Branch(dir) != "" {
		t.Error("a folder outside git has a branch")
	}
}

func TestDirty(t *testing.T) {
	status := "git --no-optional-locks -C /p status --porcelain"
	runner := &macostest.Runner{Outputs: map[string]string{status: " M main.go\n"}}
	if dirty, err := Dirty(context.Background(), runner, "/p"); err != nil || !dirty {
		t.Errorf("Dirty = %v, %v, want true", dirty, err)
	}

	runner.Outputs[status] = ""
	if dirty, _ := Dirty(context.Background(), runner, "/p"); dirty {
		t.Error("clean tree reported dirty")
	}

	runner.Errors = map[string]error{status: errors.New("not a git repository")}
	if _, err := Dirty(context.Background(), runner, "/p"); err == nil {
		t.Error("git failure hidden")
	}
}

func TestMatches(t *testing.T) {
	for _, tt := range []struct {
		name, query string
		want        bool
	}{
		{"blog/blog-app", "bba", true},
		{"museum", "MEM", true},
		{"museum", "", true},
		{"museum", "mx", false},
		{"home-app", "ppa", false},
	} {
		if got := Matches(tt.name, tt.query); got != tt.want {
			t.Errorf("Matches(%q, %q) = %v, want %v", tt.name, tt.query, got, tt.want)
		}
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "ngt", "open.json")}

	empty, err := store.Load()
	if err != nil || empty.IDE != "" || len(empty.Opened) != 0 {
		t.Fatalf("missing file = %+v, %v, want empty state", empty, err)
	}

	empty.RecordOpen("/p/museum", "/Applications/Zed.app", base)
	if err := store.Save(empty); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.IDE != "/Applications/Zed.app" || !loaded.Opened["/p/museum"].Equal(base) {
		t.Errorf("loaded = %+v", loaded)
	}
	if loaded.ProjectIDEs["/p/museum"] != "/Applications/Zed.app" {
		t.Errorf("project IDE = %q", loaded.ProjectIDEs["/p/museum"])
	}
}

func TestRecordOpenKeepsEachProjectsIDE(t *testing.T) {
	var state State
	state.RecordOpen("/p/ios-app", "/Applications/Xcode.app", base)
	state.RecordOpen("/p/web-app", "/Applications/Zed.app", base)

	if state.IDE != "/Applications/Zed.app" {
		t.Errorf("default IDE = %q, want the last one picked", state.IDE)
	}
	if state.ProjectIDEs["/p/ios-app"] != "/Applications/Xcode.app" {
		t.Errorf("ios-app IDE = %q, want Xcode", state.ProjectIDEs["/p/ios-app"])
	}
}

func TestDefaultStoreHonorsXDG(t *testing.T) {
	env := map[string]string{"XDG_CONFIG_HOME": "/xdg"}
	if got := DefaultStore("/home", func(k string) string { return env[k] }).Path; got != "/xdg/ngt/open.json" {
		t.Errorf("path = %q", got)
	}
	if got := DefaultStore("/home", func(string) string { return "" }).Path; got != "/home/.config/ngt/open.json" {
		t.Errorf("path = %q", got)
	}
}
