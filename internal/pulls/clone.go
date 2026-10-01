package pulls

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
)

// cloneTimeout is generous because a large repository can take minutes to download.
const cloneTimeout = 10 * time.Minute

// ErrFolderTaken stops a clone into a folder that already exists.
var ErrFolderTaken = errors.New("already exists")

// CloneDir is where Clone puts repo ("owner/name"): a folder named after it in root.
func CloneDir(root, repo string) string {
	return filepath.Join(root, path.Base(repo))
}

// Clone clones repo into CloneDir with gh repo clone, which follows gh's protocol choice. It
// refuses a folder that already exists, and never prompts.
func Clone(ctx context.Context, root, repo string) (string, error) {
	dir := CloneDir(root, repo)
	if _, err := os.Lstat(dir); err == nil {
		return "", ErrFolderTaken
	}
	ctx, cancel := context.WithTimeout(ctx, cloneTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "repo", "clone", repo, dir)
	cmd.Dir = root
	cmd.Env = gitstatus.NoPromptEnv(ctx, root)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", errors.New("timed out after " + cloneTimeout.String())
		}
		return "", errors.New(lastLine(stderr.String(), err.Error()))
	}
	return dir, nil
}
