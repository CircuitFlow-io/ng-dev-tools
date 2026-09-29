package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/projectlist"
)

var (
	cursor = ide.IDE{Name: "Cursor", Version: "3.22", AppPath: "/Applications/Cursor.app"}
	zed    = ide.IDE{Name: "Zed", Version: "1.20", AppPath: "/Applications/Zed.app"}
	xcode  = ide.IDE{Name: "Xcode", Version: "27.0", AppPath: "/Applications/Xcode.app"}
)

var sampleProjects = []projects.Project{
	{Name: "ng-dev-tools", Path: "/p/ng-dev-tools", Git: true, Branch: "main", Changed: time.Now()},
	{Name: "memorit", Path: "/p/memorit", Git: true, Branch: "chore/upgrade", Changed: time.Now().Add(-2 * time.Hour)},
	{Name: "multi-app", Path: "/p/multi-app", Changed: time.Now().Add(-48 * time.Hour)},
}

func scanned(t *testing.T, cfg Config) Model {
	t.Helper()
	if cfg.IDEs == nil {
		cfg.IDEs = []ide.IDE{cursor, zed, xcode}
	}
	cfg.Root, cfg.Home = "/p", "/home"
	next, _ := New(context.Background(), cfg).Update(scannedMsg{projects: sampleProjects})
	return next.(Model)
}

func press(t *testing.T, m Model, keys ...tea.KeyPressMsg) Model {
	t.Helper()
	for _, key := range keys {
		next, _ := m.Update(key)
		m = next.(Model)
	}
	return m
}

func typed(text string) []tea.KeyPressMsg {
	keys := make([]tea.KeyPressMsg, 0, len(text))
	for _, r := range text {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
)

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

func TestTypingFiltersInsteadOfMoving(t *testing.T) {
	m := press(t, scanned(t, Config{}), typed("mj")...)

	if m.list.Query() != "mj" || len(m.list.Shown()) != 0 {
		t.Errorf("query %q shows %d projects", m.list.Query(), len(m.list.Shown()))
	}
	if !strings.Contains(view(m), "No projects match") {
		t.Errorf("view = %s", view(m))
	}

	m = press(t, m, tea.KeyPressMsg{Code: tea.KeyBackspace})
	if names := len(m.list.Shown()); names != 2 {
		t.Errorf("after backspace %d projects shown, want memorit and multi-app", names)
	}
	m = press(t, m, esc)
	if m.list.Query() != "" || m.state != stateChoosingProject {
		t.Error("esc did not clear the filter")
	}
}

func TestEnterShowsIDEBoxOnTheLastUsedIDE(t *testing.T) {
	m := scanned(t, Config{State: projects.State{IDE: zed.AppPath}})
	m = press(t, m, down, enter)

	if m.state != stateChoosingIDE || m.project.Name != "memorit" {
		t.Fatalf("state %v, project %q", m.state, m.project.Name)
	}
	if m.picker.Current() != zed {
		t.Errorf("preselected %v, want Zed", m.picker.Current())
	}
	box := view(m)
	for _, want := range []string{"Open memorit with", "● Zed 1.20", "default", "○ Cursor 3.22"} {
		if !strings.Contains(box, want) {
			t.Errorf("box is missing %q:\n%s", want, box)
		}
	}
}

func TestIDEBoxPrefersTheProjectsOwnIDE(t *testing.T) {
	state := projects.State{IDE: zed.AppPath, ProjectIDEs: map[string]string{"/p/memorit": xcode.AppPath}}
	m := press(t, scanned(t, Config{State: state}), down, enter)

	if m.picker.Current() != xcode {
		t.Errorf("preselected %v, want Xcode, the IDE memorit was last opened in", m.picker.Current())
	}
	box := view(m)
	if !strings.Contains(box, "last used") || strings.Contains(box, "default") {
		t.Errorf("box should tag Xcode as last used:\n%s", box)
	}

	m = press(t, m, esc, tea.KeyPressMsg{Code: tea.KeyUp}, enter)
	if m.project.Name != "ng-dev-tools" || m.picker.Current() != zed {
		t.Errorf("%s preselected %v, want the default Zed", m.project.Name, m.picker.Current())
	}
}

func TestUninstalledProjectIDEFallsBackToTheDefault(t *testing.T) {
	state := projects.State{IDE: zed.AppPath, ProjectIDEs: map[string]string{"/p/ng-dev-tools": "/Applications/Gone.app"}}
	m := press(t, scanned(t, Config{State: state}), enter)

	if m.picker.Current() != zed || !strings.Contains(view(m), "default") {
		t.Errorf("preselected %v, want the default Zed", m.picker.Current())
	}
}

func TestChoosingAnIDEFinishes(t *testing.T) {
	m := press(t, scanned(t, Config{}), enter, tea.KeyPressMsg{Code: '3', Text: "3"})
	next, cmd := m.Update(enter)
	m = next.(Model)

	project, editor, ok := m.Chosen()
	if !ok || cmd == nil || project.Name != "ng-dev-tools" || editor != xcode {
		t.Errorf("Chosen = %q, %v, %v", project.Name, editor.Name, ok)
	}
}

func TestEscLeavesTheIDEBox(t *testing.T) {
	m := press(t, scanned(t, Config{}), enter, esc)

	if m.state != stateChoosingProject {
		t.Errorf("state = %v, want back on the project list", m.state)
	}
	if _, _, ok := m.Chosen(); ok {
		t.Error("esc chose an IDE")
	}
}

func TestSingleIDESkipsTheBox(t *testing.T) {
	m := press(t, scanned(t, Config{IDEs: []ide.IDE{cursor}}), enter)

	if _, editor, ok := m.Chosen(); !ok || editor != cursor {
		t.Errorf("Chosen = %v, %v", editor, ok)
	}
}

func TestQueryWithOneMatchGoesToTheBox(t *testing.T) {
	m := scanned(t, Config{Query: "memo"})

	if m.state != stateChoosingIDE || m.project.Name != "memorit" {
		t.Errorf("state %v, project %q", m.state, m.project.Name)
	}
}

func TestDirtyMarker(t *testing.T) {
	m := scanned(t, Config{})
	next, _ := m.Update(projectlist.DirtyMsg{Path: "/p/memorit", Dirty: true})
	m = next.(Model)

	for _, line := range strings.Split(view(m), "\n") {
		if strings.Contains(line, "memorit") && !strings.Contains(line, "●") {
			t.Errorf("memorit has no dirty marker: %q", line)
		}
		if strings.Contains(line, "ng-dev-tools") && strings.Contains(line, "●") {
			t.Errorf("clean project marked dirty: %q", line)
		}
	}
}

func TestScanErrorEndsTheFlow(t *testing.T) {
	failure := errors.New("permission denied")
	next, cmd := New(context.Background(), Config{}).Update(scannedMsg{err: failure})
	m := next.(Model)

	if !errors.Is(m.Err(), failure) || cmd == nil {
		t.Errorf("Err = %v", m.Err())
	}
}

func TestNarrowTerminalDropsThePathColumn(t *testing.T) {
	m := scanned(t, Config{})
	next, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 24})
	m = next.(Model)

	screen := view(m)
	if strings.Contains(screen, "PATH") || strings.Contains(screen, "/p/memorit") {
		t.Errorf("path column shown at 60 columns:\n%s", screen)
	}
	if !strings.Contains(screen, "chore/upgrade") {
		t.Errorf("branch missing:\n%s", screen)
	}
}
