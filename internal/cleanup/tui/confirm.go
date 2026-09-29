package tui

import (
	"fmt"
	"os/exec"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

type sudoResultMsg struct{ err error }

func (m Model) updateConfirming(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case sudoResultMsg:
		m.sudoErr = msg.err
		m.cfg.Cleaner.RootAuthorized = msg.err == nil
		return m.startCleaning()
	case tea.KeyPressMsg:
		switch msg.String() {
		case "y", "Y":
			return m.approve()
		case "n", "N", "esc", "q":
			m.state = stateSelecting
		}
	}
	return m, nil
}

// approve asks for the administrator password first when root-owned items were chosen.
// sudo caches the credentials, so the cleaner can then use `sudo -n` without prompting.
func (m Model) approve() (tea.Model, tea.Cmd) {
	rootCount := countRootItems(m.chosen)
	if rootCount == 0 || m.cfg.Cleaner.DryRun {
		return m.startCleaning()
	}
	prompt := fmt.Sprintf("ngt needs administrator access to delete %d root-owned item(s).\nPassword: ", rootCount)
	sudo := exec.Command("sudo", "-v", "-p", prompt)
	return m, tea.ExecProcess(sudo, func(err error) tea.Msg { return sudoResultMsg{err: err} })
}

func (m Model) confirmView() string {
	size := ui.Bytes(cleanup.TotalSize(m.chosen))
	lines := []string{
		ui.Danger.Bold(true).Render(fmt.Sprintf("Permanently delete %s (%s)?", ui.Count(len(m.chosen), "item"), size)),
		"",
		"This cannot be undone. Nothing is moved to the Trash.",
	}
	if m.cfg.Cleaner.DryRun {
		lines[0] = ui.Title.Render(fmt.Sprintf("Dry run: pretend to delete %s (%s)?", ui.Count(len(m.chosen), "item"), size))
		lines[2] = "Nothing will be deleted; the log shows what would have been removed."
	}
	if rootCount := countRootItems(m.chosen); rootCount > 0 && !m.cfg.Cleaner.DryRun {
		lines = append(lines, ui.Warning.Render(fmt.Sprintf("%d item(s) are owned by root; you will be asked for your password.", rootCount)))
	}
	lines = append(lines, "", ui.Bold.Render("y")+" confirm   "+ui.Bold.Render("n")+" back to the list")

	box := ui.WarnBox
	if m.cfg.Cleaner.DryRun {
		box = ui.Box
	}
	return box.Render(strings.Join(lines, "\n"))
}

func countRootItems(items []cleanup.Item) int {
	var count int
	for _, item := range items {
		if item.NeedsRoot && len(item.RemoveCommand) == 0 {
			count++
		}
	}
	return count
}
