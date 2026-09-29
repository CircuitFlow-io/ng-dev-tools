package tui

import (
	"image/color"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/projectlist"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/scripts"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	recentTitle = "Recent"
	rootSuffix  = " (root)"
	scriptArrow = " › "
)

// group is one entry of the package pane: a package, or the recent runs across packages.
type group struct {
	title   string
	recent  bool
	targets []scripts.Target
}

// scriptPicker is the script screen: a package pane, the focused package's scripts, and a search
// over every package that replaces both panes while typing.
type scriptPicker struct {
	ws             scripts.Workspace
	recentRuns     []scripts.Run
	groups         []group
	groupCursor    ui.ListCursor
	cursor         ui.ListCursor
	query          string
	results        []scripts.Target
	resultCursor   ui.ListCursor
	nodes          map[string]scripts.Node
	missingModules bool
	width          int
	height         int
	highlight      color.Color
}

func newScriptPicker(ws scripts.Workspace, recent []scripts.Run, currentDir, nvmDir string) scriptPicker {
	p := scriptPicker{
		ws:             ws,
		recentRuns:     recent,
		nodes:          nodesByPackage(ws, nvmDir),
		missingModules: ws.MissingModules(),
		height:         1,
	}
	p.groups = buildGroups(ws, recent)
	p.groupCursor = ui.NewListCursor(len(p.groups), p.initialGroup(currentDir))
	p.focusGroup()
	return p
}

func nodesByPackage(ws scripts.Workspace, nvmDir string) map[string]scripts.Node {
	nodes := map[string]scripts.Node{}
	for _, pkg := range ws.Packages {
		nodes[pkg.Dir] = scripts.RequiredNode(pkg.Dir, ws.Root, nvmDir)
	}
	return nodes
}

func buildGroups(ws scripts.Workspace, recent []scripts.Run) []group {
	var groups []group
	if targets := recentTargets(ws, recent); len(targets) > 0 && hasPackagePane(ws) {
		groups = append(groups, group{title: recentTitle, recent: true, targets: targets})
	}
	for _, pkg := range ws.Packages {
		targets := make([]scripts.Target, len(pkg.Scripts))
		for i, s := range pkg.Scripts {
			targets[i] = scripts.Target{Package: pkg, Script: s}
		}
		groups = append(groups, group{title: packageTitle(ws, pkg), targets: targets})
	}
	return groups
}

// recentTargets resolves the recent runs that still exist.
func recentTargets(ws scripts.Workspace, recent []scripts.Run) []scripts.Target {
	var targets []scripts.Target
	for _, run := range recent {
		if t, ok := ws.Find(run.Package, run.Script); ok {
			targets = append(targets, t)
		}
	}
	return targets
}

func hasPackagePane(ws scripts.Workspace) bool {
	return len(ws.Packages) > 1
}

func packageTitle(ws scripts.Workspace, pkg scripts.Package) string {
	if pkg.IsRoot() {
		return ws.Name() + rootSuffix
	}
	return pkg.RelDir
}

// targetLabel names a script with its package, for lists mixing packages.
func targetLabel(t scripts.Target) string {
	if t.Package.IsRoot() {
		return t.Script.Name
	}
	return t.Package.RelDir + scriptArrow + t.Script.Name
}

// initialGroup focuses the package holding currentDir, then the recent runs, then the root.
func (p scriptPicker) initialGroup(currentDir string) int {
	if pkg, ok := p.ws.PackageFor(currentDir); ok && !pkg.IsRoot() {
		i := slices.IndexFunc(p.groups, func(g group) bool { return !g.recent && g.targets[0].Package.Dir == pkg.Dir })
		if i >= 0 {
			return i
		}
	}
	return 0
}

func (p scriptPicker) focused() group {
	return p.groups[p.groupCursor.Index]
}

// focusGroup shows the focused group's scripts, with the cursor on the one run most recently.
func (p *scriptPicker) focusGroup() {
	p.cursor = ui.NewListCursor(len(p.focused().targets), p.lastRunIndex(p.focused()))
	p.cursor.Resize(p.height)
}

func (p scriptPicker) lastRunIndex(g group) int {
	if g.recent {
		return 0
	}
	for _, run := range p.recentRuns {
		i := slices.IndexFunc(g.targets, func(t scripts.Target) bool {
			return t.Package.RelDir == run.Package && t.Script.Name == run.Script
		})
		if i >= 0 {
			return i
		}
	}
	return 0
}

func (p scriptPicker) searching() bool {
	return p.query != ""
}

func (p *scriptPicker) setQuery(query string) {
	p.query = query
	p.results = nil
	if query != "" {
		p.results = scripts.Search(p.ws.Targets(), p.recentRuns, query)
	}
	p.resultCursor = ui.NewListCursor(len(p.results), 0)
	p.resultCursor.Resize(p.height)
}

// current is the script under the cursor.
func (p scriptPicker) current() (scripts.Target, bool) {
	if p.searching() {
		if len(p.results) == 0 {
			return scripts.Target{}, false
		}
		return p.results[p.resultCursor.Index], true
	}
	targets := p.focused().targets
	if len(targets) == 0 {
		return scripts.Target{}, false
	}
	return targets[p.cursor.Index], true
}

// handleKey applies a search edit, a package switch or a move, and reports whether it was one.
func (p *scriptPicker) handleKey(key tea.KeyPressMsg) bool {
	switch key.String() {
	case "backspace":
		if !p.searching() {
			return false
		}
		p.setQuery(dropLastRune(p.query))
		return true
	case "tab", "right":
		return p.moveGroup(1)
	case "shift+tab", "left":
		return p.moveGroup(-1)
	}
	if projectlist.IsTyping(key) {
		p.setQuery(p.query + key.Text)
		return true
	}
	if p.searching() {
		return p.resultCursor.HandleKey(key.String())
	}
	return p.cursor.HandleKey(key.String())
}

// moveGroup focuses the next or previous package, wrapping around.
func (p *scriptPicker) moveGroup(delta int) bool {
	if p.searching() || len(p.groups) < 2 {
		return false
	}
	p.groupCursor.MoveTo((p.groupCursor.Index + delta + len(p.groups)) % len(p.groups))
	p.focusGroup()
	return true
}

func (p *scriptPicker) resize(width, height int) {
	p.width = width
	p.height = max(height, 1)
	p.groupCursor.Resize(p.height)
	p.cursor.Resize(p.height)
	p.resultCursor.Resize(p.height)
}
