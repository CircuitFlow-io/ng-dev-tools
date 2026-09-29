package doctor

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

const sshKeyFix = `ssh-keygen -t ed25519 && gh ssh-key add ~/.ssh/id_ed25519.pub`

var (
	ghAccountPattern  = regexp.MustCompile(`account (\S+)`)
	sshAccountPattern = regexp.MustCompile(`Hi ([^!]+)!`)
)

func gitChecks() []Check {
	return []Check{
		{Name: "Git identity", Group: GroupGit, Run: checkGitIdentity},
		{Name: "Git defaults", Group: GroupGit, Run: checkGitDefaults},
		{Name: "GitHub CLI", Group: GroupGit, NeedsNetwork: true, Run: checkGitHubCLI},
		{Name: "GitHub SSH", Group: GroupGit, NeedsNetwork: true, Run: checkGitHubSSH},
	}
}

// gitConfig reads a global setting; git exits non-zero when it is unset.
func (e Env) gitConfig(ctx context.Context, key string) string {
	value, _ := e.output(ctx, "git", "config", "--global", key)
	return value
}

func checkGitIdentity(ctx context.Context, env Env) Result {
	if !env.installed("git") {
		return fail("git is not installed", "xcode-select --install")
	}
	name, email := env.gitConfig(ctx, "user.name"), env.gitConfig(ctx, "user.email")
	if name == "" || email == "" {
		return fail("user.name or user.email is not set", `git config --global user.name "Your Name" && git config --global user.email you@example.com`)
	}
	return pass(fmt.Sprintf("%s <%s>", name, email))
}

// gitDefault is a global setting worth having, with the value to suggest.
type gitDefault struct {
	key       string
	suggested string
}

var gitDefaults = []gitDefault{
	{"init.defaultBranch", "main"},
	{"pull.rebase", "true"},
}

func checkGitDefaults(ctx context.Context, env Env) Result {
	if !env.installed("git") {
		return skip("git is not installed")
	}
	var set, fixes []string
	for _, d := range gitDefaults {
		if value := env.gitConfig(ctx, d.key); value != "" {
			set = append(set, d.key+"="+value)
			continue
		}
		fixes = append(fixes, fmt.Sprintf("git config --global %s %s", d.key, d.suggested))
	}
	if len(fixes) > 0 {
		return warn(fmt.Sprintf("%d not set", len(fixes)), strings.Join(fixes, " && "))
	}
	return pass(strings.Join(set, ", "))
}

func checkGitHubCLI(ctx context.Context, env Env) Result {
	if !env.installed("gh") {
		return fail("not installed", "brew install gh")
	}
	out, err := env.output(ctx, "gh", "auth", "status")
	if err != nil {
		return fail("not logged in", "gh auth login")
	}
	if match := ghAccountPattern.FindStringSubmatch(out); match != nil {
		return pass("logged in as " + match[1])
	}
	return pass("logged in")
}

func checkGitHubSSH(ctx context.Context, env Env) Result {
	keys, _ := filepath.Glob(env.homePath(".ssh", "id_*.pub"))
	if len(keys) == 0 {
		return fail("no SSH key", sshKeyFix)
	}
	// GitHub answers `ssh -T` with exit status 1 even on success, so the verdict is in the message.
	out, err := env.output(ctx, "ssh", "-T", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=yes", "git@github.com")
	message := out
	if err != nil {
		message += "\n" + err.Error()
	}
	return judgeGitHubSSH(message)
}

func judgeGitHubSSH(message string) Result {
	switch {
	case strings.Contains(message, "successfully authenticated"):
		if match := sshAccountPattern.FindStringSubmatch(message); match != nil {
			return pass("authenticated as " + match[1])
		}
		return pass("authenticated")
	case strings.Contains(message, "Host key verification failed"):
		return warn("github.com is not in known_hosts", "ssh -T git@github.com and accept the host key")
	case strings.Contains(message, "Permission denied"):
		return fail("GitHub rejected your SSH key", "gh ssh-key add ~/.ssh/id_ed25519.pub")
	default:
		return warn("could not reach GitHub over SSH", "").with(lastLine(message))
	}
}

func lastLine(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}
