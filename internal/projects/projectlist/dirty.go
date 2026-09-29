package projectlist

import (
	"context"

	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
	"github.com/nasserghiasi/ng-dev-tools/internal/projects"
)

// DirtyMsg reports whether the project at Path has uncommitted changes.
type DirtyMsg struct {
	Path  string
	Dirty bool
}

// DirtyChecks looks for uncommitted changes in the background, one command per git project.
func DirtyChecks(ctx context.Context, runner macos.Runner, found []projects.Project) tea.Cmd {
	var cmds []tea.Cmd
	for _, p := range found {
		if !p.Git {
			continue
		}
		cmds = append(cmds, func() tea.Msg {
			dirty, err := projects.Dirty(ctx, runner, p.Path)
			if err != nil {
				return nil
			}
			return DirtyMsg{Path: p.Path, Dirty: dirty}
		})
	}
	return tea.Batch(cmds...)
}
