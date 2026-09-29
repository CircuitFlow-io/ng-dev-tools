package cli

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
)

func TestPrintStatusListsOnlyGitRepositories(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	for _, name := range []string{"draft", "plain"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "notes.txt"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", filepath.Join(root, "draft"), "init", "--quiet", "--initial-branch=main").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}

	var out bytes.Buffer
	if err := printStatus(context.Background(), &out, macos.ExecRunner{}, root, false); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("want a header and one repository, got:\n%s", out.String())
	}
	if fields := strings.Fields(lines[1]); strings.Join(fields, " ") != "draft main ?1 no remote no commits" {
		t.Errorf("row = %q", lines[1])
	}
}
