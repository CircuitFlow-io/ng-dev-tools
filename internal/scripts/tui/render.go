package tui

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/nasserghiasi/ng-dev-tools/internal/scripts"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

const (
	cursorWidth      = 2
	columnGap        = 2
	countWidth       = 4
	minPaneWidth     = 18
	maxPaneWidth     = 40
	minNameWidth     = 8
	maxNameWidth     = 36
	maxPackageWidth  = 30
	minCommandWidth  = 12
	detailFrameWidth = 4
	// detailLines is the fixed height of the details box's content, so the lists above do not jump.
	detailLines     = 7
	maxCommandLines = 3
	referenceIndent = 2
	runPrompt       = "$ "
)

var (
	scriptStyle = lipgloss.NewStyle().Foreground(ui.ColorAccent)
	detailBox   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.ColorAccent).Padding(0, 1)
)

func dropLastRune(s string) string {
	_, size := utf8.DecodeLastRuneInString(s)
	return s[:len(s)-size]
}

func (p scriptPicker) paneWidth() int {
	if !hasPackagePane(p.ws) {
		return 0
	}
	longest := 0
	for _, g := range p.groups {
		longest = max(longest, lipgloss.Width(g.title))
	}
	return min(maxPaneWidth, max(minPaneWidth, cursorWidth+longest+countWidth+columnGap))
}

func (p scriptPicker) header() string {
	if p.searching() {
		return ui.Muted.Render(fmt.Sprintf("  %s for %q", ui.Count(len(p.results), "match"), p.query))
	}
	scriptsTitle := "SCRIPTS"
	if hasPackagePane(p.ws) {
		scriptsTitle += " in " + p.focused().title
	}
	return ui.Muted.Render(ui.PadRight("  PACKAGES", p.paneWidth())) + ui.Muted.Render("  "+scriptsTitle)
}

// rows renders the lists, always p.height lines tall so the details box below stays in place.
func (p scriptPicker) rows() string {
	var lines []string
	switch {
	case p.searching():
		lines = p.resultRows()
	case hasPackagePane(p.ws):
		lines = p.paneRows()
	default:
		lines = p.scriptRows()
	}
	for len(lines) < p.height {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (p scriptPicker) paneRows() []string {
	right := p.scriptRows()
	left := p.groupRows()
	lines := make([]string, max(len(left), len(right)))
	for i := range lines {
		l, r := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		lines[i] = ui.PadRight(l, p.paneWidth()) + r
	}
	return lines
}

func (p scriptPicker) groupRows() []string {
	width := p.paneWidth()
	titleWidth := width - cursorWidth - countWidth - columnGap
	start, end := p.groupCursor.Visible()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		g := p.groups[i]
		title := ui.PadRight(ui.Truncate(g.title, titleWidth), titleWidth)
		count := ui.Muted.Render(ui.PadLeft(strconv.Itoa(len(g.targets)), countWidth))
		switch {
		case i == p.groupCursor.Index:
			lines = append(lines, ui.Selected.Render("▸ "+title)+count)
		case g.recent:
			lines = append(lines, "  "+ui.Heading.Render(title)+count)
		default:
			lines = append(lines, "  "+title+count)
		}
	}
	return lines
}

func (p scriptPicker) scriptRows() []string {
	g := p.focused()
	label := func(t scripts.Target) string {
		if g.recent {
			return targetLabel(t)
		}
		return t.Script.Name
	}
	nameWidth := fitWidth(g.targets, label, minNameWidth, maxNameWidth)
	width := p.width - p.paneWidth()
	commandWidth := max(minCommandWidth, width-cursorWidth-nameWidth)

	start, end := p.cursor.Visible()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		t := g.targets[i]
		painter := ui.NewRowPainter(i == p.cursor.Index, p.highlight)
		row := painter.Cursor() +
			painter.Paint(nameStyle(painter).Width(nameWidth), ui.Truncate(label(t), nameWidth-columnGap)) +
			painter.Paint(ui.Muted, ui.Truncate(t.Script.Command, commandWidth-1))
		lines = append(lines, painter.Fill(row, width))
	}
	return lines
}

func (p scriptPicker) resultRows() []string {
	if len(p.results) == 0 {
		return []string{ui.Muted.Render("  No scripts match")}
	}
	packageWidth := fitWidth(p.results, func(t scripts.Target) string { return packageTitle(p.ws, t.Package) }, minNameWidth, maxPackageWidth)
	nameWidth := fitWidth(p.results, func(t scripts.Target) string { return t.Script.Name }, minNameWidth, maxNameWidth)
	commandWidth := max(minCommandWidth, p.width-cursorWidth-packageWidth-nameWidth)

	start, end := p.resultCursor.Visible()
	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		t := p.results[i]
		painter := ui.NewRowPainter(i == p.resultCursor.Index, p.highlight)
		row := painter.Cursor() +
			painter.Paint(lipgloss.NewStyle().Width(packageWidth), ui.Truncate(packageTitle(p.ws, t.Package), packageWidth-columnGap)) +
			painter.Paint(nameStyle(painter).Width(nameWidth), ui.Truncate(t.Script.Name, nameWidth-columnGap)) +
			painter.Paint(ui.Muted, ui.Truncate(t.Script.Command, commandWidth-1))
		lines = append(lines, painter.Fill(row, p.width))
	}
	return lines
}

func nameStyle(painter ui.RowPainter) lipgloss.Style {
	if painter.IsHighlighted() {
		return scriptStyle.Bold(true)
	}
	return scriptStyle
}

func fitWidth(targets []scripts.Target, field func(scripts.Target) string, least, most int) int {
	longest := 0
	for _, t := range targets {
		longest = max(longest, lipgloss.Width(field(t)))
	}
	return min(most, max(least, longest+columnGap))
}

// details explains the script under the cursor: its command, what it calls and how it will run.
func (p scriptPicker) details() string {
	width := p.width - detailFrameWidth
	t, ok := p.current()
	if !ok {
		return detailBox.Width(p.width).Render(padLines([]string{ui.Muted.Render("No script selected")}))
	}
	lines := []string{ui.Bold.Render(targetLabel(t))}
	lines = append(lines, wrap(t.Script.Command, width, maxCommandLines)...)
	room := detailLines - len(lines) - 1
	notes := p.notes(t, width)
	lines = append(lines, notes[:min(len(notes), max(room, 0))]...)
	lines = append(lines, p.runLine(t, width))
	return detailBox.Width(p.width).Render(padLines(lines))
}

// notes are what the command alone does not say, most important first.
func (p scriptPicker) notes(t scripts.Target, width int) []string {
	var notes []string
	manager := string(p.ws.Manager)
	if p.missingModules {
		notes = append(notes, ui.Warning.Render(ui.Truncate("node_modules is missing: run "+manager+" install first", width)))
	}
	if node := p.nodes[t.Package.Dir]; node.Spec != "" && !node.Installed() {
		notes = append(notes, ui.Warning.Render(ui.Truncate(fmt.Sprintf("%s asks for Node %s, which nvm does not have: nvm install %s", node.Source, node.Spec, node.Spec), width)))
	}
	notes = append(notes, p.hookNotes(t.Script, width)...)
	for _, ref := range scripts.References(t.Package, t.Script) {
		indent := strings.Repeat(" ", (ref.Depth-1)*referenceIndent)
		name := indent + "↳ " + ref.Script.Name + "  "
		notes = append(notes, ui.Muted.Render(name)+ui.Truncate(ref.Script.Command, width-lipgloss.Width(name)))
	}
	return notes
}

func (p scriptPicker) hookNotes(s scripts.Script, width int) []string {
	hook := func(prefix, command, when string) string {
		name := prefix + s.Name
		if !p.ws.RunsHooks {
			return ui.Warning.Render(ui.Truncate(name+" is skipped: pnpm runs pre/post scripts only with enablePrePostScripts", width))
		}
		return ui.Muted.Render(ui.Truncate("runs "+name+" "+when+": "+command, width))
	}
	var notes []string
	if s.Pre != "" {
		notes = append(notes, hook("pre", s.Pre, "first"))
	}
	if s.Post != "" {
		notes = append(notes, hook("post", s.Post, "after"))
	}
	return notes
}

func (p scriptPicker) runLine(t scripts.Target, width int) string {
	line := runPrompt + scripts.DisplayCommand(p.ws, t)
	if node := p.nodes[t.Package.Dir]; node.Installed() {
		line += "  · Node " + node.Version + " from " + node.Source
	}
	return ui.Muted.Render(ui.Truncate(line, width))
}

// wrap breaks text into at most limit lines of width, ending the last with … when cut short.
func wrap(text string, width, limit int) []string {
	lines := strings.Split(ansi.Wrap(text, max(width, 1), " "), "\n")
	if len(lines) <= limit {
		return lines
	}
	lines = lines[:limit]
	lines[limit-1] = ui.Truncate(lines[limit-1]+" …", width)
	return lines
}

func padLines(lines []string) string {
	for len(lines) < detailLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
