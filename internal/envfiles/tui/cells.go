package tui

import (
	"slices"
	"strconv"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	noteJoiner   = " · "
	pipeMark     = " (pipe)"
	noCount      = "–"
	unknownCount = "?"
	defaultBase  = ".env"
)

var (
	accent = lipgloss.NewStyle().Foreground(ui.ColorAccent)
	plain  = lipgloss.NewStyle()
)

type glyph struct {
	symbol string
	style  lipgloss.Style
}

var glyphs = map[envfiles.Level]glyph{
	envfiles.Exposed:      {"✖", ui.Danger},
	envfiles.Missing:      {"●", ui.Warning},
	envfiles.Empty:        {"○", ui.Warning},
	envfiles.Undocumented: {"◆", accent},
	envfiles.NotSetUp:     {"◦", ui.Muted},
	envfiles.NoExample:    {"◦", ui.Muted},
	envfiles.Unchecked:    {"◦", ui.Muted},
	envfiles.Complete:     {"✓", ui.Success},
}

func glyphCell(s envfiles.Set) ui.Cell {
	g := glyphs[s.Attention()]
	return ui.Cell{ui.NewSpan(g.symbol, g.style)}
}

// folderCell names the folder, and the local file the example describes when it is not .env,
// which tells apart two examples in one folder.
func folderCell(s envfiles.Set, nameStyle lipgloss.Style) ui.Cell {
	c := ui.Cell{ui.NewSpan(s.Name, nameStyle)}
	if base := s.Base(); base != "" && base != defaultBase {
		c = append(c, ui.NewSpan(" ("+base+")", ui.Muted))
	}
	return c
}

func filesCell(s envfiles.Set) ui.Cell {
	switch {
	case len(s.Locals) > 0:
		return localFilesCell(s)
	case s.Example != "":
		return ui.Cell{ui.NewSpan("no "+s.Base()+" yet", ui.Muted)}
	}
	return ui.Cell{ui.NewSpan(noCount, ui.Muted)}
}

// localFilesCell lists the local files, marking the ones that are pipes and so are not read.
func localFilesCell(s envfiles.Set) ui.Cell {
	var parts []ui.Cell
	for _, file := range s.Locals {
		c := ui.Cell{ui.NewSpan(file, plain)}
		if slices.Contains(s.Unread, file) {
			c = append(c, ui.NewSpan(pipeMark, ui.Muted))
		}
		parts = append(parts, c)
	}
	return ui.JoinCells(parts, ", ")
}

func countCell(n int, style lipgloss.Style) ui.Cell {
	if n == 0 {
		return ui.Cell{ui.NewSpan(noCount, ui.Muted)}
	}
	return ui.Cell{ui.NewSpan(strconv.Itoa(n), style)}
}

func missingCell(s envfiles.Set) ui.Cell { return unknownOrCount(s, len(s.Missing), ui.Warning) }
func extraCell(s envfiles.Set) ui.Cell   { return countCell(len(s.Extra), accent) }
func emptyCell(s envfiles.Set) ui.Cell   { return unknownOrCount(s, len(s.Empty), ui.Warning) }

// unknownOrCount shows ? when a local file could not be read, since its keys may be the ones
// counted as missing or empty.
func unknownOrCount(s envfiles.Set, n int, style lipgloss.Style) ui.Cell {
	if len(s.Unread) > 0 {
		return ui.Cell{ui.NewSpan(unknownCount, ui.Muted)}
	}
	return countCell(n, style)
}

func notesCell(s envfiles.Set) ui.Cell {
	var parts []ui.Cell
	if s.Err != nil {
		parts = append(parts, ui.Cell{ui.NewSpan("unreadable", ui.Danger)})
	}
	if len(s.Tracked) > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan("tracked by git", ui.Danger)})
	}
	if len(s.Committed) > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan("in git history", ui.Danger)})
	}
	if n := len(s.InCode); n > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan(strconv.Itoa(n)+" used in code", accent)})
	}
	if len(s.Unread) > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan("not read", ui.Muted)})
	}
	if s.Example == "" && len(s.Locals) > 0 {
		parts = append(parts, ui.Cell{ui.NewSpan("no example", ui.Muted)})
	}
	return ui.JoinCells(parts, noteJoiner)
}

// PlainRow is a set's table row as plain text, for output that is not a terminal.
func PlainRow(s envfiles.Set) []string {
	return []string{
		folderCell(s, plain).Text(),
		filesCell(s).Text(),
		missingCell(s).Text(),
		extraCell(s).Text(),
		emptyCell(s).Text(),
		notesCell(s).Text(),
	}
}
