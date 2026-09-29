// Package idepicker is the select box for the IDE to open a project with, shared by the open and
// status screens.
package idepicker

import (
	"image/color"
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	help             = "↑/↓ move · 1-9 pick · enter open · esc back"
	defaultTag       = "default"
	lastUsedTag      = "last used"
	radioOn          = "● "
	radioOff         = "○ "
	radioWidth       = 2
	numberWidth      = 3
	labelPadding     = 3
	maxNumberedIDEs  = 9
	firstNumberedKey = 1
)

// Picker is the IDE select box.
type Picker struct {
	ides        []ide.IDE
	preselected string
	tag         string
	cursor      int
	highlight   color.Color
}

// New starts on the IDE the project was last opened in, or else the one picked last for any
// project, so enter repeats the previous choice. An IDE no longer installed is skipped.
func New(ides []ide.IDE, projectApp, defaultApp string) Picker {
	p := Picker{ides: ides}
	if !p.preselect(projectApp, lastUsedTag) {
		p.preselect(defaultApp, defaultTag)
	}
	return p
}

func (p *Picker) preselect(appPath, tag string) bool {
	index := slices.IndexFunc(p.ides, func(editor ide.IDE) bool { return editor.AppPath == appPath })
	if appPath == "" || index < 0 {
		return false
	}
	p.cursor, p.preselected, p.tag = index, appPath, tag
	return true
}

// SetHighlight sets the background of the cursor row.
func (p *Picker) SetHighlight(c color.Color) {
	p.highlight = c
}

// Current is the IDE under the cursor.
func (p Picker) Current() ide.IDE {
	return p.ides[p.cursor]
}

// HandleKey applies a navigation key and reports whether it was one.
func (p *Picker) HandleKey(key string) bool {
	switch key {
	case "up", "k":
		p.cursor = max(0, p.cursor-1)
	case "down", "j":
		p.cursor = min(len(p.ides)-1, p.cursor+1)
	case "home", "g":
		p.cursor = 0
	case "end", "G":
		p.cursor = len(p.ides) - 1
	default:
		return p.jumpTo(key)
	}
	return true
}

// jumpTo moves to the IDE numbered key, counting from 1.
func (p *Picker) jumpTo(key string) bool {
	n, err := strconv.Atoi(key)
	if err != nil || n < firstNumberedKey || n > min(len(p.ides), maxNumberedIDEs) {
		return false
	}
	p.cursor = n - firstNumberedKey
	return true
}

// View renders the box for opening project.
func (p Picker) View(project string) string {
	width := p.rowWidth()
	lines := []string{ui.Title.Render("Open " + project + " with"), ""}
	for i := range p.ides {
		lines = append(lines, p.renderRow(i, width))
	}
	lines = append(lines, "", ui.Muted.Render(help))
	return ui.Box.Render(strings.Join(lines, "\n"))
}

func (p Picker) labelWidth() int {
	longest := 0
	for _, editor := range p.ides {
		longest = max(longest, lipgloss.Width(editor.Label()))
	}
	return longest + labelPadding
}

func (p Picker) rowWidth() int {
	return numberWidth + radioWidth + p.labelWidth() + lipgloss.Width(p.tag)
}

func (p Picker) renderRow(index, width int) string {
	editor := p.ides[index]
	painter := ui.NewRowPainter(index == p.cursor, p.highlight)

	number := painter.Paint(ui.Muted.Width(numberWidth), "")
	if index < maxNumberedIDEs {
		number = painter.Paint(ui.Muted.Width(numberWidth), strconv.Itoa(index+firstNumberedKey))
	}
	radio := painter.Paint(ui.Muted, radioOff)
	labelStyle := lipgloss.NewStyle().Width(p.labelWidth())
	if painter.IsHighlighted() {
		radio = painter.Paint(ui.Selected, radioOn)
		labelStyle = labelStyle.Bold(true)
	}
	tag := ""
	if editor.AppPath == p.preselected {
		tag = painter.Paint(ui.Success, p.tag)
	}
	return painter.Fill(number+radio+painter.Paint(labelStyle, editor.Label())+tag, width)
}
