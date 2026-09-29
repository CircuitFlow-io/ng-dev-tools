package standup

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

func TestLocalWorkWithRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	dir := filepath.Join(root, "projects", "api")
	runGit(t, root, "init", "--quiet", "--bare", "--initial-branch=main", remote)
	runGit(t, root, "clone", "--quiet", remote, dir)
	runGit(t, dir, "config", "user.email", "Me@Example.com")

	since := time.Now().Add(-48 * time.Hour)
	longAgo := since.Add(-72 * time.Hour).Format(time.RFC3339)
	recently := since.Add(time.Hour).Format(time.RFC3339)
	commitAs(t, dir, "me@example.com", longAgo, longAgo, "old work")
	commitAs(t, dir, "kim@example.com", recently, recently, "kim's work")
	commitAs(t, dir, "me@example.com", recently, recently, "on main")
	runGit(t, dir, "push", "--quiet", "origin", "main")
	runGit(t, dir, "remote", "set-head", "origin", "main")

	runGit(t, dir, "switch", "--quiet", "-c", "feat/pushed")
	commitAs(t, dir, "me@example.com", recently, recently, "pushed feature")
	runGit(t, dir, "push", "--quiet", "origin", "feat/pushed")
	runGit(t, dir, "switch", "--quiet", "-c", "feat/stacked")
	commitAs(t, dir, "me@example.com", recently, recently, "stacked feature")
	runGit(t, dir, "switch", "--quiet", "main")
	commitAs(t, dir, "me@example.com", longAgo, recently, "rebased old work")

	runner := macos.ExecRunner{}
	r := readRepo(context.Background(), runner, "api", dir)
	if r.defaultBranch != "main" || r.defaultRef != "origin/main" {
		t.Fatalf("default branch = %q, %q", r.defaultBranch, r.defaultRef)
	}
	work := r.load(context.Background(), runner, since)
	var got []string
	for _, c := range work.commits {
		got = append(got, c.Subject+" @"+work.branchOf[c.Hash])
	}
	slices.Sort(got)
	want := []string{"on main @", "pushed feature @feat/pushed", "stacked feature @feat/stacked"}
	if !slices.Equal(got, want) {
		t.Errorf("commits = %q, want %q: yours only, by author date, each on its own branch", got, want)
	}

	times := r.commitTimes(context.Background(), runner, time.Now())
	if len(times) != 5 {
		t.Errorf("commitTimes = %v, want all 5 of your commits in the lookback", times)
	}
}

func TestUnpushedCommitsGetNoURL(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	dir := filepath.Join(root, "api")
	runGit(t, root, "init", "--quiet", "--bare", "--initial-branch=main", remote)
	runGit(t, root, "clone", "--quiet", remote, dir)
	runGit(t, dir, "config", "user.email", "me@example.com")
	now := time.Now().Format(time.RFC3339)
	commitAs(t, dir, "me@example.com", now, now, "pushed")
	runGit(t, dir, "push", "--quiet", "origin", "main")
	commitAs(t, dir, "me@example.com", now, now, "local")

	r := readRepo(context.Background(), macos.ExecRunner{}, "api", dir)
	r.github = "acme/api"
	work := r.load(context.Background(), macos.ExecRunner{}, time.Now().Add(-time.Hour))
	urls := map[string]string{}
	for _, c := range work.commits {
		urls[c.Subject] = c.URL
	}
	if urls["local"] != "" || urls["pushed"] == "" {
		t.Errorf("urls = %q, want a link for the pushed commit only", urls)
	}
}

func commitAs(t *testing.T, dir, email, authored, committed, message string) {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "commit", "--quiet", "--allow-empty", "-m", message)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Someone", "GIT_AUTHOR_EMAIL="+email, "GIT_AUTHOR_DATE="+authored,
		"GIT_COMMITTER_NAME=Someone", "GIT_COMMITTER_EMAIL="+email, "GIT_COMMITTER_DATE="+committed)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
		t.Fatalf("git %q: %v\n%s", args, err, out)
	}
}

func TestWorktreesCountAsOneRepository(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	root := t.TempDir()
	runGit(t, root, "init", "--quiet", "--initial-branch=main", "b-api")
	main := filepath.Join(root, "b-api")
	now := time.Now().Format(time.RFC3339)
	commitAs(t, main, "me@example.com", now, now, "first")
	runGit(t, main, "worktree", "add", "--quiet", "-b", "feat/x", filepath.Join(root, "a-api-feature"))
	runGit(t, root, "init", "--quiet", "other")
	if err := os.Mkdir(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}

	dirs := []string{filepath.Join(root, "a-api-feature"), main, filepath.Join(root, "notes"), filepath.Join(root, "other")}
	got := oneCheckoutPerRepository(dirs)
	want := []string{main, filepath.Join(root, "other")}
	if !slices.Equal(got, want) {
		t.Errorf("checkouts = %q, want %q: the main checkout, not its worktree, and no folder without git", got, want)
	}
}
