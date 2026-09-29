package doctor

import (
	"context"
	"strings"
)

const (
	claudeCodePackage = "@anthropic-ai/claude-code"
	homebrewPrefix    = "/opt/homebrew/"
)

func claudeChecks() []Check {
	return []Check{{Name: "Claude Code", Group: GroupClaude, Run: checkClaudeCode}}
}

func checkClaudeCode(ctx context.Context, env Env) Result {
	current, err := env.version(ctx, "claude", "--version")
	if err != nil {
		return missing(err, "brew install --cask claude-code")
	}
	latest, err := env.Releases.Npm(ctx, claudeCodePackage)
	return compareWithLatest(current, latest, err, claudeUpdateFix(env))
}

// claudeUpdateFix matches the update command to how Claude Code was installed.
func claudeUpdateFix(env Env) string {
	path, _ := env.LookPath("claude")
	if strings.HasPrefix(path, homebrewPrefix) {
		return "brew upgrade claude-code"
	}
	return "claude update"
}
