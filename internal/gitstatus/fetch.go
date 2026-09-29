package gitstatus

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	fetchTimeout  = 30 * time.Second
	defaultSSH    = "ssh"
	sshNoPrompts  = " -o BatchMode=yes"
	noTermPrompts = "GIT_TERMINAL_PROMPT=0"
)

// Fetch runs git fetch in dir, which only updates remote-tracking branches. It never asks for a
// password or passphrase: a prompt would take over the terminal behind the interface, so a fetch
// that needs one fails instead.
func Fetch(ctx context.Context, dir string) error {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "fetch", "--quiet")
	cmd.Env = NoPromptEnv(ctx, dir)
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return errors.New("timed out after " + fetchTimeout.String())
		}
		return errors.New(firstLine(stderr.String(), err.Error()))
	}
	return nil
}

// NoPromptEnv is the environment for a git command in dir that must fail rather than ask for a
// password or passphrase, which would take over the terminal behind an interface.
func NoPromptEnv(ctx context.Context, dir string) []string {
	env := append(os.Environ(), noTermPrompts)
	if ssh, ok := batchSSHCommand(ctx, dir); ok {
		env = append(env, "GIT_SSH_COMMAND="+ssh)
	}
	return env
}

// batchSSHCommand is the ssh command git would use, with prompts turned off. It is left alone
// when GIT_SSH names a program that may not take ssh's options.
func batchSSHCommand(ctx context.Context, dir string) (string, bool) {
	if os.Getenv("GIT_SSH") != "" {
		return "", false
	}
	ssh := os.Getenv("GIT_SSH_COMMAND")
	if ssh == "" {
		out, _ := exec.CommandContext(ctx, "git", "-C", dir, "config", "--get", "core.sshCommand").Output()
		ssh = strings.TrimSpace(string(out))
	}
	if ssh == "" {
		ssh = defaultSSH
	}
	return ssh + sshNoPrompts, true
}

func firstLine(text, fallback string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if line == "" {
		return fallback
	}
	return line
}
