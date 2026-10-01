package tui

import (
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	noteJoiner = " · "
	you        = "you"
)

var (
	accent       = lipgloss.NewStyle().Foreground(ui.ColorAccent)
	plain        = lipgloss.NewStyle()
	markerStyles = map[todos.Marker]lipgloss.Style{
		todos.Fixme: ui.Danger.Bold(true),
		todos.Hack:  ui.Warning.Bold(true),
		todos.Todo:  accent.Bold(true),
	}
)

func ageCell(item todos.Item, now time.Time) ui.Cell {
	if item.Uncommitted {
		return ui.Cell{ui.NewSpan("not committed", ui.Muted)}
	}
	return ui.Cell{ui.NewSpan(ui.Age(now, item.At), plain)}
}

func markerCell(item todos.Item) ui.Cell {
	return ui.Cell{ui.NewSpan(string(item.Marker), markerStyles[item.Marker])}
}

func noteCell(item todos.Item) ui.Cell {
	if item.Note == "" {
		return ui.Cell{ui.NewSpan("no note", ui.Muted)}
	}
	return ui.TicketCell(item.Note, plain)
}

func projectCell(item todos.Item) ui.Cell {
	return ui.Cell{ui.NewSpan(item.Project, plain)}
}

// fileCell keeps the end of a long path, where the file name is.
func fileCell(item todos.Item, width int) ui.Cell {
	return ui.Cell{ui.NewSpan(ui.TruncatePath(item.Location(), width), ui.Muted)}
}

func authorCell(item todos.Item) ui.Cell {
	if item.Mine {
		return ui.Cell{ui.NewSpan(you, accent)}
	}
	return ui.Cell{ui.NewSpan(item.Author, plain)}
}

// PlainRow is an item's table row as plain text, for output that is not a terminal.
func PlainRow(item todos.Item, now time.Time) []string {
	return []string{
		ageCell(item, now).Text(),
		markerCell(item).Text(),
		item.Project,
		item.Location(),
		authorCell(item).Text(),
		noteCell(item).Text(),
	}
}
