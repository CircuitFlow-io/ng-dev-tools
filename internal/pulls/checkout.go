package pulls

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const checkoutTimeout = time.Minute

// ErrUncommitted stops a checkout that would carry uncommitted changes onto another branch.
var ErrUncommitted = errors.New("has uncommitted changes: commit or stash them first")

// Checkout switches the clone in dir to the pull request's branch with gh pr checkout, which
// fetches it, forks included. It refuses to when dir has uncommitted changes, and never prompts.
func Checkout(ctx context.Context, runner macos.Runner, dir string, p PR) error {
	dirty, err := projects.Dirty(ctx, runner, dir)
	if err != nil {
		return err
	}
	if dirty {
		return ErrUncommitted
	}

	ctx, cancel := context.WithTimeout(ctx, checkoutTimeout)
	defer cancel()
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "pr", "checkout", strconv.Itoa(p.Number), "--repo", p.Repo)
	cmd.Dir = dir
	cmd.Env = gitstatus.NoPromptEnv(ctx, dir)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("timed out after " + checkoutTimeout.String())
		}
		return errors.New(lastLine(stderr.String(), err.Error()))
	}
	return nil
}

// lastLine is the last line of a command's error output, where gh and git put the reason.
func lastLine(text, fallback string) string {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return last
	}
	return fallback
}
