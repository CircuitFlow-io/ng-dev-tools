package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const selectingHelp = "↑/↓ move · space toggle · c category · a all · n none · enter continue · q quit"

func (m Model) updateSelecting(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return m, nil
	}
	if m.list.cursor.HandleKey(key.String()) {
		return m, nil
	}
	switch key.String() {
	case "space", "x":
		m.list.toggle()
	case "c":
		m.list.toggleCurrentCategory()
	case "a":
		m.list.setAll(true)
	case "n":
		m.list.setAll(false)
	case "enter":
		return m.confirmSelection()
	case "q", "esc":
		return m.finish(true)
	}
	return m, nil
}

func (m Model) confirmSelection() (tea.Model, tea.Cmd) {
	m.chosen = m.list.selectedItems()
	if len(m.chosen) == 0 {
		return m, nil
	}
	m.state = stateConfirming
	return m, nil
}

func (m Model) selectingView() string {
	chosen := m.list.selectedItems()
	lines := []string{
		ui.Title.Render("Found "+ui.Bytes(cleanup.TotalSize(m.list.items))+" of reclaimable space") +
			ui.Muted.Render("  ("+ui.Count(len(m.list.items), "item")+")"),
		"",
		m.list.view(m.now),
		"",
		m.currentItemDetail(),
		fmt.Sprintf("%s %s", ui.Bold.Render(fmt.Sprintf("%d selected", len(chosen))),
			ui.Success.Render(ui.Bytes(cleanup.TotalSize(chosen)))) + m.warningNote(),
		ui.Help.Render(selectingHelp),
	}
	return strings.Join(lines, "\n")
}

func (m Model) currentItemDetail() string {
	item, ok := m.list.currentItem()
	if !ok {
		return ui.Muted.Render("space toggles the whole category")
	}
	return ui.Muted.Render(ui.TruncatePath(item.Detail, m.width-2))
}

func (m Model) warningNote() string {
	if len(m.warnings) == 0 {
		return ""
	}
	return ui.Warning.Render(fmt.Sprintf("   ⚠ %d source(s) skipped, details at the end", len(m.warnings)))
}
