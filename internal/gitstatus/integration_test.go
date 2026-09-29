package gitstatus

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
)

// sandbox is a folder of real repositories sharing one bare remote, with git's user and system
// configuration shut out.
type sandbox struct {
	t    *testing.T
	root string
}

func newSandbox(t *testing.T) sandbox {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	s := sandbox{t: t, root: t.TempDir()}
	s.git(s.root, "init", "--quiet", "--bare", "--initial-branch=main", "remote.git")
	return s
}

func (s sandbox) git(dir string, args ...string) {
	s.t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		s.t.Fatalf("git %q: %v\n%s", args, err, out)
	}
}

// clone makes a working copy of the remote in the projects folder.
func (s sandbox) clone(name string) string {
	s.t.Helper()
	s.git(filepath.Join(s.root, "projects"), "clone", "--quiet", filepath.Join(s.root, "remote.git"), name)
	return filepath.Join(s.root, "projects", name)
}

func (s sandbox) commit(dir, file, content string) {
	s.t.Helper()
	s.write(dir, file, content)
	s.git(dir, "add", file)
	s.git(dir, "commit", "--quiet", "-m", "change "+file)
}

func (s sandbox) write(dir, file, content string) {
	s.t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(content), 0o644); err != nil {
		s.t.Fatal(err)
	}
}

func (s sandbox) load(dir string) Repo {
	s.t.Helper()
	r := Load(context.Background(), macos.ExecRunner{}, filepath.Base(dir), dir)
	if r.Err != nil {
		s.t.Fatalf("Load(%s): %v", dir, r.Err)
	}
	return r
}

func TestLoadReadsRealRepositories(t *testing.T) {
	s := newSandbox(t)
	if err := os.MkdirAll(filepath.Join(s.root, "projects"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := s.clone("mine")
	s.commit(mine, "app.txt", "one\n")
	s.git(mine, "push", "--quiet", "-u", "origin", "main")
	theirs := s.clone("theirs")

	if r := s.load(mine); r.Attention() != Clean || r.Upstream != "origin/main" || r.LastCommit.Subject != "change app.txt" {
		t.Fatalf("a freshly pushed clone should be clean: %+v", r)
	}

	s.commit(theirs, "app.txt", "two\n")
	s.git(theirs, "push", "--quiet")
	if err := Fetch(context.Background(), mine); err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if r := s.load(mine); r.Behind != 1 || r.Attention() != Behind || r.FetchedAt.IsZero() {
		t.Errorf("after fetching, mine should be 1 behind: %+v", r)
	}

	s.commit(mine, "app.txt", "three\n")
	if r := s.load(mine); r.Ahead != 1 || r.Unpushed != 1 || len(r.UnpushedCommits) != 1 {
		t.Errorf("a local commit should be unpushed: %+v", r)
	}

	s.write(mine, "app.txt", "stashed\n")
	s.git(mine, "stash", "--quiet")
	s.git(mine, "branch", "spike")
	s.git(mine, "push", "--quiet", "origin", "HEAD:doomed")
	s.git(mine, "branch", "--quiet", "--set-upstream-to=origin/doomed", "spike")
	s.git(theirs, "push", "--quiet", "origin", "--delete", "doomed")
	s.git(mine, "fetch", "--quiet", "--prune")
	r := s.load(mine)
	if len(r.Stashes) != 1 || r.Stashes[0].Ref != "stash@{0}" {
		t.Errorf("stashes = %+v", r.Stashes)
	}
	if !slices.ContainsFunc(r.Branches, func(b Branch) bool { return b.Name == "spike" && b.Gone }) {
		t.Errorf("spike's remote branch was deleted: %+v", r.Branches)
	}

	if err := exec.Command("git", "-C", mine, "merge", "--quiet", "origin/main").Run(); err == nil {
		t.Fatal("the merge should have stopped on a conflict")
	}
	r = s.load(mine)
	if r.Operation != Merge || r.Changes().Conflicted != 1 || r.Attention() != Conflict {
		t.Errorf("mid-merge: operation %v, changes %+v, level %v", r.Operation, r.Changes(), r.Attention())
	}
}

func TestLoadAllSkipsFoldersWithoutGit(t *testing.T) {
	s := newSandbox(t)
	projects := filepath.Join(s.root, "projects")
	for _, dir := range []string{"local", "plain"} {
		if err := os.MkdirAll(filepath.Join(projects, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	s.write(filepath.Join(projects, "plain"), "notes.txt", "no git here")
	s.git(filepath.Join(projects, "local"), "init", "--quiet")
	s.write(filepath.Join(projects, "local"), "draft.txt", "untracked")

	repos, err := LoadAll(context.Background(), macos.ExecRunner{}, projects)
	if err != nil {
		t.Fatal(err)
	}
	if len(repos) != 1 || repos[0].Name != "local" {
		t.Fatalf("repos = %+v, want only local", repos)
	}
	r := repos[0]
	if !r.Unborn || r.HasRemote || r.Changes().Untracked != 1 || r.Attention() != Uncommitted {
		t.Errorf("local = %+v", r)
	}
}

func TestFetchReportsWhyItFailed(t *testing.T) {
	s := newSandbox(t)
	dir := filepath.Join(s.root, "orphan")
	s.git(s.root, "init", "--quiet", "orphan")
	s.git(dir, "remote", "add", "origin", filepath.Join(s.root, "missing.git"))
	err := Fetch(context.Background(), dir)
	if err == nil || err.Error() == "" {
		t.Fatalf("Fetch from a missing remote = %v, want an error with git's message", err)
	}
}
