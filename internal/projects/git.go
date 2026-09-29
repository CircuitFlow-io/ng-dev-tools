package projects

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	shortSHALength  = 7
	branchRefPrefix = "ref: refs/heads/"
	gitDirPrefix    = "gitdir: "
)

// Branch is the checked-out branch, the short commit for a detached HEAD, or "" outside git.
// It reads .git/HEAD instead of running git, so listing many projects stays fast.
func Branch(dir string) string {
	git := GitDir(dir)
	if git == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(git, "HEAD"))
	if err != nil {
		return ""
	}
	head := strings.TrimSpace(string(data))
	if branch, ok := strings.CutPrefix(head, branchRefPrefix); ok {
		return branch
	}
	if len(head) >= shortSHALength {
		return head[:shortSHALength]
	}
	return ""
}

// Dirty reports whether dir has uncommitted changes. --no-optional-locks keeps git from
// rewriting the index, so looking never counts as a change.
func Dirty(ctx context.Context, runner macos.Runner, dir string) (bool, error) {
	out, err := runner.Run(ctx, "git", "--no-optional-locks", "-C", dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return len(bytes.TrimSpace(out)) > 0, nil
}

// GitDir is the repository folder for dir, following the "gitdir:" file of worktrees and
// submodules, or "" when dir is not a repository.
func GitDir(dir string) string {
	path := filepath.Join(dir, ".git")
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	if info.IsDir() {
		return path
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), gitDirPrefix)
	if !ok {
		return ""
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(dir, target)
	}
	return target
}
