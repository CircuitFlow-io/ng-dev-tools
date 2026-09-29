package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/cleanup"
	"github.com/nasserghiasi/ng-dev-tools/internal/macos/macostest"
)

const mb = 1 << 20

func sampleItems() []cleanup.Item {
	return []cleanup.Item{
		{Title: "DerivedData", Category: cleanup.CategoryDeveloper, Safety: cleanup.SafetySafe, Size: 300 * mb, RemoveCommand: []string{"true"}},
		{Title: "Old archive", Category: cleanup.CategoryDeveloper, Safety: cleanup.SafetyReview, Size: 200 * mb, RemoveCommand: []string{"true"}},
		{Title: "Figma", Category: cleanup.CategoryApps, Safety: cleanup.SafetyReview, Size: 100 * mb, RemoveCommand: []string{"true"}},
	}
}

func newTestModel(t *testing.T, dryRun bool) Model {
	t.Helper()
	cfg := Config{
		Scanner: cleanup.Scanner{Env: cleanup.Env{Now: time.Now()}},
		Cleaner: cleanup.Cleaner{Guard: cleanup.NewGuard(t.TempDir()), Runner: &macostest.Runner{}, DryRun: dryRun},
	}
	return New(context.Background(), cfg)
}

func press(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		next, _ := m.Update(keyMsg(k))
		m = next.(Model)
	}
	return m
}

func keyMsg(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

func scanned(t *testing.T, m Model, items []cleanup.Item) Model {
	t.Helper()
	next, _ := m.Update(scanDoneMsg{Items: items})
	return next.(Model)
}

func TestSafeItemsArePreselected(t *testing.T) {
	m := scanned(t, newTestModel(t, false), sampleItems())

	chosen := m.list.selectedItems()
	if len(chosen) != 1 || chosen[0].Title != "DerivedData" {
		t.Errorf("preselected = %v, want only the safe item", chosen)
	}
}

func TestSelectionKeys(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want int
	}{
		{"select all", []string{"a"}, 3},
		{"select none", []string{"n"}, 0},
		{"toggle the item under the cursor", []string{"space"}, 0},
		{"toggle a review item", []string{"down", "space"}, 2},
		{"category toggles every item in it", []string{"c"}, 2},
		{"category toggle clears a fully selected category", []string{"c", "c"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := press(t, scanned(t, newTestModel(t, false), sampleItems()), tt.keys...)
			if got := len(m.list.selectedItems()); got != tt.want {
				t.Errorf("selected %d items, want %d", got, tt.want)
			}
		})
	}
}

func TestFlowFromSelectionToSummary(t *testing.T) {
	m := scanned(t, newTestModel(t, true), sampleItems())

	m = press(t, m, "enter")
	if m.state != stateConfirming || !strings.Contains(m.View().Content, "Dry run") {
		t.Fatalf("state = %v, want the dry-run confirmation", m.state)
	}

	m = press(t, m, "n")
	if m.state != stateSelecting {
		t.Fatalf("state = %v, want back to selecting", m.state)
	}

	m = press(t, m, "enter", "y")
	if m.state != stateCleaning {
		t.Fatalf("state = %v, want cleaning", m.state)
	}

	next, cmd := m.Update(cleanDoneMsg{{Item: m.chosen[0]}})
	m = next.(Model)
	if m.state != stateDone || cmd == nil {
		t.Fatalf("state = %v, want done with a quit command", m.state)
	}
	if view := m.View().Content; !strings.Contains(view, "would free 315 MB") {
		t.Errorf("summary = %q", view)
	}
}

func TestEnterWithNothingSelectedStays(t *testing.T) {
	m := press(t, scanned(t, newTestModel(t, false), sampleItems()), "n", "enter")
	if m.state != stateSelecting {
		t.Errorf("state = %v, want selecting", m.state)
	}
}

func TestEmptyScanFinishes(t *testing.T) {
	m := scanned(t, newTestModel(t, false), nil)
	if m.state != stateDone || !strings.Contains(m.View().Content, "already tidy") {
		t.Errorf("state = %v, view = %q", m.state, m.View().Content)
	}
}

func TestRootItemsRequireSudoBeforeCleaning(t *testing.T) {
	items := sampleItems()
	items[0].NeedsRoot, items[0].RemoveCommand = true, nil
	m := press(t, scanned(t, newTestModel(t, false), items), "enter")

	next, cmd := m.Update(keyMsg("y"))
	m = next.(Model)
	if m.state != stateConfirming || cmd == nil {
		t.Fatalf("state = %v, want to stay confirming while sudo runs", m.state)
	}

	next, _ = m.Update(sudoResultMsg{})
	m = next.(Model)
	if m.state != stateCleaning || !m.cfg.Cleaner.RootAuthorized {
		t.Errorf("state = %v, authorized = %v", m.state, m.cfg.Cleaner.RootAuthorized)
	}
}
