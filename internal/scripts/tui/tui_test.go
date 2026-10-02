package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/scripts"
)

const (
	monorepoRoot = "/p/museum"
	singleRoot   = "/p/weather"
)

var (
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	esc   = tea.KeyPressMsg{Code: tea.KeyEscape}
	up    = tea.KeyPressMsg{Code: tea.KeyUp}
	down  = tea.KeyPressMsg{Code: tea.KeyDown}
	left  = tea.KeyPressMsg{Code: tea.KeyLeft}
	right = tea.KeyPressMsg{Code: tea.KeyRight}
)

func monorepo() scripts.Workspace {
	return scripts.Workspace{Root: monorepoRoot, Manager: scripts.PNPM, Packages: []scripts.Package{
		{Dir: monorepoRoot, Scripts: []scripts.Script{{Name: "build", Command: "turbo build"}, {Name: "lint", Command: "turbo lint"}}},
		{Dir: monorepoRoot + "/apps/native", RelDir: "apps/native", Scripts: []scripts.Script{{Name: "ios", Command: "expo run:ios"}}},
		{Dir: monorepoRoot + "/apps/web", RelDir: "apps/web", Scripts: []scripts.Script{{Name: "dev", Command: "next dev"}, {Name: "test", Command: "vitest"}}},
	}}
}

func singlePackage() scripts.Workspace {
	return scripts.Workspace{Root: singleRoot, Manager: scripts.NPM, Packages: []scripts.Package{
		{Dir: singleRoot, Scripts: []scripts.Script{{Name: "start", Command: "node ."}, {Name: "test", Command: "jest"}}},
	}}
}

var sampleProjects = []projects.Project{
	{Name: "weather", Path: singleRoot, Changed: time.Now()},
	{Name: "museum", Path: monorepoRoot, Changed: time.Now().Add(-time.Hour)},
}

func monorepoHistory(runs ...scripts.Run) scripts.History {
	return scripts.History{Runs: map[string][]scripts.Run{monorepoRoot: runs}}
}

// started is the screen once the projects were scanned and the selected project's scripts read.
func started(t *testing.T, cfg Config) Model {
	t.Helper()
	cfg.Root, cfg.Home = "/p", "/home"
	m := update(t, New(context.Background(), cfg), scannedMsg{projects: sampleProjects})
	return readScripts(t, m)
}

func readScripts(t *testing.T, m Model) Model {
	t.Helper()
	project, ok := m.projects.current()
	if !ok {
		return m
	}
	ws := map[string]scripts.Workspace{monorepoRoot: monorepo(), singleRoot: singlePackage()}[project.Path]
	return update(t, m, loadedMsg{root: project.Path, workspace: ws})
}

func update(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func typed(text string) []tea.Msg {
	keys := make([]tea.Msg, 0, len(text))
	for _, r := range text {
		keys = append(keys, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return keys
}

func current(t *testing.T, m Model) string {
	t.Helper()
	if m.picker == nil {
		t.Fatal("no scripts shown")
	}
	target, ok := m.picker.current()
	if !ok {
		return ""
	}
	return target.Package.RelDir + ":" + target.Script.Name
}

func view(m Model) string {
	return ansi.Strip(m.View().Content)
}

func TestStartsOnTheLastRunScriptReadyToRun(t *testing.T) {
	history := monorepoHistory(scripts.Run{Package: "apps/web", Script: "test", At: time.Now()})
	m := started(t, Config{History: history})

	if project, _ := m.projects.current(); project.Path != monorepoRoot {
		t.Errorf("project = %q, want the one a script last ran in", project.Path)
	}
	if m.focus != focusScripts || current(t, m) != "apps/web:test" {
		t.Fatalf("focus %v on %q, want the scripts column on apps/web:test", m.focus, current(t, m))
	}

	m = update(t, m, enter)
	ws, target, ok := m.Chosen()
	if !ok || ws.Root != monorepoRoot || target.Script.Name != "test" {
		t.Errorf("Chosen = %q %q %v", ws.Root, target.Script.Name, ok)
	}
}

func TestWithoutHistoryStartsOnTheProjects(t *testing.T) {
	m := started(t, Config{})

	if m.focus != focusProjects {
		t.Errorf("focus = %v, want the projects column", m.focus)
	}
	if !strings.Contains(view(m), "PROJECTS") || !strings.Contains(view(m), "SCRIPTS") {
		t.Errorf("columns missing:\n%s", view(m))
	}
}

func TestEnterStepsThroughTheColumns(t *testing.T) {
	m := started(t, Config{})
	m = readScripts(t, update(t, m, down))
	if m.picker == nil || m.picker.ws.Root != monorepoRoot {
		t.Fatal("moving down did not show the monorepo's scripts")
	}

	m = update(t, m, enter)
	if m.focus != focusPackages {
		t.Fatalf("focus = %v after enter on a project, want packages", m.focus)
	}
	m = update(t, m, down, down, enter)
	if m.focus != focusScripts || current(t, m) != "apps/web:dev" {
		t.Fatalf("focus %v on %q, want the scripts of apps/web", m.focus, current(t, m))
	}
	if _, _, ok := m.Chosen(); ok {
		t.Fatal("ran before enter on a script")
	}
	if _, _, ok := update(t, m, enter).Chosen(); !ok {
		t.Error("enter on a script did not run it")
	}
}

func TestArrowsMoveBetweenColumnsAndStopAtTheEnds(t *testing.T) {
	history := monorepoHistory(scripts.Run{Package: "apps/web", Script: "dev", At: time.Now()})
	m := started(t, Config{History: history})

	steps := []struct {
		key  tea.KeyPressMsg
		want focus
	}{
		{left, focusPackages},
		{left, focusProjects},
		{left, focusProjects},
		{right, focusPackages},
		{right, focusScripts},
		{right, focusScripts},
	}
	for _, step := range steps {
		m = update(t, m, step.key)
		if m.focus != step.want {
			t.Fatalf("after %s focus = %v, want %v", step.key, m.focus, step.want)
		}
	}
}

func TestSinglePackageProjectHasNoPackageColumn(t *testing.T) {
	m := started(t, Config{})
	m = update(t, m, right)

	if m.focus != focusScripts {
		t.Errorf("focus = %v, want scripts right after projects", m.focus)
	}
	if strings.Contains(view(m), "PACKAGES") {
		t.Errorf("packages column shown for a single package:\n%s", view(m))
	}
}

func TestUpDownInPackagesSelectsTheLastRunScriptOfEach(t *testing.T) {
	history := monorepoHistory(
		scripts.Run{Package: "", Script: "lint", At: time.Now()},
		scripts.Run{Package: "apps/web", Script: "test", At: time.Now().Add(-time.Hour)},
	)
	m := started(t, Config{History: history})
	if current(t, m) != ":lint" {
		t.Fatalf("starts on %q, want the root's lint", current(t, m))
	}

	m = update(t, m, left, down, down, down)
	if current(t, m) != "apps/web:test" {
		t.Errorf("apps/web starts on %q, want its last run script", current(t, m))
	}
}

func TestTypingFiltersProjectsOrSearchesScriptsByColumn(t *testing.T) {
	history := monorepoHistory(scripts.Run{Package: "apps/web", Script: "dev", At: time.Now()})
	m := started(t, Config{History: history})

	m = update(t, m, typed("ios")...)
	if m.picker.query != "ios" || current(t, m) != "apps/native:ios" {
		t.Fatalf("search %q on %q", m.picker.query, current(t, m))
	}
	m = update(t, m, esc)
	if m.picker.searching() {
		t.Fatal("esc did not clear the search")
	}

	m = update(t, m, left, left)
	m = readScripts(t, update(t, m, typed("wea")...))
	if project, _ := m.projects.current(); project.Path != singleRoot || m.picker.ws.Root != singleRoot {
		t.Errorf("filter selected %q", project.Path)
	}
}

func TestStartedInsideAProjectSelectsItAndThePackageHere(t *testing.T) {
	ws := monorepo()
	cfg := Config{Root: "/p", Workspace: &ws, CurrentDir: monorepoRoot + "/apps/native/src"}
	m := New(context.Background(), cfg)
	if m.focus != focusScripts || current(t, m) != "apps/native:ios" {
		t.Fatalf("before the scan: focus %v on %q", m.focus, current(t, m))
	}

	m = update(t, m, scannedMsg{projects: sampleProjects[:1]})
	if project, _ := m.projects.current(); project.Path != monorepoRoot || len(m.projects.all) != 2 {
		t.Errorf("project %q of %d, want the current one added and selected", project.Path, len(m.projects.all))
	}
	if current(t, m) != "apps/native:ios" {
		t.Errorf("the scan moved the selection to %q", current(t, m))
	}
}

func TestQueryMatchingOneProjectGoesToItsScripts(t *testing.T) {
	m := started(t, Config{Query: "mus"})

	if m.focus != focusScripts || m.picker == nil || m.picker.ws.Root != monorepoRoot {
		t.Errorf("focus %v, want the museum's scripts", m.focus)
	}
}

func TestLoadingAnotherProjectShowsTheSpinner(t *testing.T) {
	m := update(t, started(t, Config{}), down)

	if m.picker != nil || !strings.Contains(view(m), "Reading the scripts of museum") {
		t.Errorf("no loading state:\n%s", view(m))
	}
	m = update(t, m, up)
	if m.picker == nil || m.picker.ws.Root != singleRoot {
		t.Error("going back did not reuse the scripts already read")
	}
}
