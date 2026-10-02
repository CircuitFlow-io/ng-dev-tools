package cli

import (
	"context"
	"io"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

// claudeUsedVerb names a project's last Claude session in the activity column.
const claudeUsedVerb = "claude"

type claudeStartFlags struct {
	root string
}

func runClaudeStart(ctx context.Context, out io.Writer, mode outputMode, query string, flags claudeStartFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := projectsRoot(ctx, flags.root, home)
	used, err := lastClaudeSessions(home, root)
	if err != nil {
		return err
	}

	switch mode {
	case outputText:
		return printProjects(ctx, out, root, home, query, used, claudeUsedVerb)
	case outputJSON:
		found, err := findProjects(ctx, root, query, used)
		if err != nil {
			return err
		}
		return writeJSON(out, toClaudeProjectsJSON(found))
	}

	launcher := &tui.Launcher{
		Title:       "Start Claude in a project",
		Help:        "type to filter · ↑/↓ move · enter start claude · esc clear or quit",
		WindowTitle: "ngt claude",
		UsedVerb:    claudeUsedVerb,
		Used:        used,
	}
	cfg := tui.Config{Root: root, Home: home, Query: query, Runner: macos.ExecRunner{}, Launcher: launcher}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	m := final.(tui.Model)
	if err := m.Err(); err != nil {
		return err
	}
	project, _, ok := m.Chosen()
	if !ok {
		return nil
	}
	lipgloss.Fprintln(out, ui.Selected.Render("▸ ")+"Starting Claude in "+ui.Bold.Render(project.Name))
	return claudesessions.Start(project.Path)
}

// lastClaudeSessions maps each project in root to when a Claude session in it was last active.
func lastClaudeSessions(home, root string) (map[string]time.Time, error) {
	dirs, err := projects.Dirs(root)
	if err != nil {
		return nil, err
	}
	return claudesessions.LastActiveIn(claudesessions.Dir(home, os.Getenv), dirs), nil
}
