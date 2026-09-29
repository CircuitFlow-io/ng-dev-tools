package gitstatus

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const sampleStatus = "# branch.oid 0123456789abcdef\x00# branch.head main\x00# branch.upstream origin/main\x00# branch.ab +2 -1\x00" +
	"1 M. N... 100644 100644 100644 h1 h2 staged.go\x00" +
	"1 .M N... 100644 100644 100644 h1 h2 dir/with space.go\x00" +
	"1 MM N... 100644 100644 100644 h1 h2 both.go\x00" +
	"2 R. N... 100644 100644 100644 h1 h2 R100 new.go\x00old.go\x00" +
	"u UU N... 100644 100644 100644 100644 h1 h2 h3 conflict.go\x00" +
	"? notes.txt\x00"

func repoFromStatus(out string) Repo {
	var r Repo
	r.applyStatus(parseStatus([]byte(out)))
	return r
}

func TestParseStatusReadsBranchAndFiles(t *testing.T) {
	r := repoFromStatus(sampleStatus)
	if r.Branch != "main" || r.Upstream != "origin/main" || r.Ahead != 2 || r.Behind != 1 || r.UpstreamGone {
		t.Errorf("branch = %q upstream %q +%d -%d gone %v", r.Branch, r.Upstream, r.Ahead, r.Behind, r.UpstreamGone)
	}
	want := Changes{Staged: 3, Modified: 2, Untracked: 1, Conflicted: 1}
	if got := r.Changes(); got != want {
		t.Errorf("Changes() = %+v, want %+v", got, want)
	}
	paths := make([]string, len(r.Files))
	for i, f := range r.Files {
		paths[i] = f.Path
	}
	if !slices.Equal(paths, []string{"staged.go", "dir/with space.go", "both.go", "new.go", "conflict.go", "notes.txt"}) {
		t.Errorf("paths = %q", paths)
	}
	if renamed := r.Files[3]; renamed.From != "old.go" || renamed.Code() != "R " {
		t.Errorf("rename = %+v code %q", renamed, renamed.Code())
	}
	if code := r.Files[5].Code(); code != "??" {
		t.Errorf("untracked code = %q", code)
	}
}

func TestParseStatusSpecialHeads(t *testing.T) {
	unborn := repoFromStatus("# branch.oid (initial)\x00# branch.head main\x00")
	if !unborn.Unborn || unborn.Branch != "main" {
		t.Errorf("unborn = %+v", unborn)
	}
	detached := repoFromStatus("# branch.oid 0123456789abcdef\x00# branch.head (detached)\x00")
	if !detached.Detached || detached.Branch != "0123456" {
		t.Errorf("detached = %+v", detached)
	}
	gone := repoFromStatus("# branch.oid 0123456789abcdef\x00# branch.head fix\x00# branch.upstream origin/fix\x00")
	if !gone.UpstreamGone || gone.HasUpstream() {
		t.Errorf("an upstream without ahead/behind counts should be gone: %+v", gone)
	}
}

func TestParseBranchesKeepsNoteworthyOnesNewestFirst(t *testing.T) {
	out := "*\x00current\x00\x00\x001700000900\n" +
		" \x00synced\x00origin/synced\x00\x001700000800\n" +
		" \x00old\x00origin/old\x00behind 2\x001700000100\n" +
		" \x00mixed\x00origin/mixed\x00ahead 3, behind 1\x001700000200\n" +
		" \x00spike\x00\x00\x001700000300\n" +
		" \x00fix\x00origin/fix\x00gone\x001700000050\n"
	branches := parseBranches(out)
	names := make([]string, len(branches))
	for i, b := range branches {
		names[i] = b.Name
	}
	if !slices.Equal(names, []string{"spike", "mixed", "old", "fix"}) {
		t.Fatalf("branches = %q", names)
	}
	if b := branches[1]; b.Ahead != 3 || b.Behind != 1 {
		t.Errorf("mixed = %+v", b)
	}
	if !branches[0].NotPushed() || !branches[3].Gone {
		t.Errorf("spike should be unpushed and fix gone: %+v", branches)
	}
	want := BranchCounts{Behind: 2, Ahead: 1, NotPushed: 1, Gone: 1}
	if got := (Repo{Branches: branches}).BranchCounts(); got != want {
		t.Errorf("BranchCounts() = %+v, want %+v", got, want)
	}
}

func TestDetectOperation(t *testing.T) {
	tests := []struct {
		markers []string
		want    Operation
	}{
		{nil, NoOperation},
		{[]string{"MERGE_HEAD"}, Merge},
		{[]string{"rebase-merge/", "MERGE_HEAD"}, Rebase},
		{[]string{"rebase-apply/"}, Rebase},
		{[]string{"CHERRY_PICK_HEAD"}, CherryPick},
		{[]string{"REVERT_HEAD"}, Revert},
		{[]string{"BISECT_LOG"}, Bisect},
	}
	for _, tt := range tests {
		dir := t.TempDir()
		for _, marker := range tt.markers {
			touch(t, dir, marker)
		}
		if got := detectOperation(dir); got != tt.want {
			t.Errorf("markers %q: got %v, want %v", tt.markers, got, tt.want)
		}
	}
	if Rebase.Verb() != "rebasing" || Rebase.Finish() != "git rebase --continue or --abort" {
		t.Errorf("Rebase describes itself as %q, %q", Rebase.Verb(), Rebase.Finish())
	}
}

func TestFetchedAtFollowsAWorktreeToTheMainRepository(t *testing.T) {
	main := t.TempDir()
	touch(t, main, fetchHeadFile)
	worktree := filepath.Join(main, "worktrees", "feature")
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "commondir"), []byte("../..\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if fetchedAt(worktree).IsZero() {
		t.Error("the worktree should report the main repository's last fetch")
	}
	if !fetchedAt(t.TempDir()).IsZero() {
		t.Error("a repository never fetched should report no time")
	}
}

func TestAttentionAndSort(t *testing.T) {
	now := time.Now()
	repos := []Repo{
		{Name: "clean", HasRemote: true, Upstream: "origin/main"},
		{Name: "stashed", HasRemote: true, Upstream: "origin/main", Stashes: []Stash{{Ref: "stash@{0}"}}},
		{Name: "unpushed", HasRemote: true, Upstream: "origin/main", Unpushed: 2},
		{Name: "never-pushed", HasRemote: true},
		{Name: "dirty-old", Files: []File{{Kind: Untracked}}, LastCommit: Commit{At: now.Add(-time.Hour)}},
		{Name: "dirty-new", Files: []File{{Kind: Untracked}}, LastCommit: Commit{At: now}},
		{Name: "behind", Behind: 1, Files: []File{{Kind: Untracked}}},
		{Name: "rebasing", Operation: Rebase},
		{Name: "broken", Err: errors.New("not a git repository")},
		{Name: "local-only"},
	}
	Sort(repos)
	var names []string
	for _, r := range repos {
		names = append(names, r.Name)
	}
	want := []string{"broken", "rebasing", "behind", "dirty-new", "dirty-old", "never-pushed", "unpushed", "stashed", "clean", "local-only"}
	if !slices.Equal(names, want) {
		t.Errorf("sorted = %q\nwant     %q", names, want)
	}
	if Stale.NeedsAttention() || !Unpushed.NeedsAttention() {
		t.Error("only levels above Stale should need attention")
	}
}

// touch creates an empty file, or a folder when name ends with a slash.
func touch(t *testing.T, dir, name string) {
	t.Helper()
	if folder, ok := strings.CutSuffix(name, "/"); ok {
		if err := os.MkdirAll(filepath.Join(dir, folder), 0o755); err != nil {
			t.Fatal(err)
		}
		return
	}
	if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
		t.Fatal(err)
	}
}
