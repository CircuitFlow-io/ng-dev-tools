// Package macos wraps the macOS command-line tools ngt relies on.
package macos

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Runner executes external commands. It exists so callers can be tested with fakes.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
	Available(name string) bool
}

// ExecRunner runs commands with os/exec.
type ExecRunner struct{}

// Run executes name with args and returns stdout. A failing command's stderr is included in the error.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Available reports whether name is found on PATH.
func (ExecRunner) Available(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
