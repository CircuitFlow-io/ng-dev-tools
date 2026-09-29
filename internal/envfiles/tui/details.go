package tui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// detailLines is the fixed height of the details box's content, so the table does not jump.
	detailLines      = 8
	detailFrameWidth = 4
	// twoColumnWidth is the terminal width from which what you lack sits beside what the example lacks.
	twoColumnWidth  = 100
	columnSeparator = " │ "
)

var detailBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.ColorAccent).Padding(0, 1)

// details describes the set under the cursor, by key name only.
func details(s envfiles.Set, ok bool, now time.Time, width int) string {
	inner := width - detailFrameWidth
	if !ok {
		return detailBox.Width(width).Render(padLines([]string{ui.Muted.Render("No env files found")}))
	}
	lines := []string{ui.FitLine(headerLine(s), inner)}
	for _, alert := range alerts(s, now) {
		lines = append(lines, ui.FitLine(alert, inner))
	}
	if len(s.Unread) > 0 {
		lines = append(lines, ui.FitLine(ui.Muted.Render(unreadNote(s)), inner))
	}
	if room := detailLines - len(lines); room > 0 {
		lines = append(lines, body(s, inner, room)...)
	}
	return detailBox.Width(width).Render(padLines(lines[:min(len(lines), detailLines)]))
}

func headerLine(s envfiles.Set) string {
	line := ui.Bold.Render(s.Name) + "  " + filesCell(s).String()
	switch {
	case s.Example == "" && len(s.Locals) == 0:
		return line + ui.Muted.Render(noteJoiner+"deleted")
	case s.Example == "":
		return line + ui.Muted.Render(noteJoiner+"no example to compare with")
	}
	return line + ui.Muted.Render(fmt.Sprintf("%s%s lists %s", noteJoiner, s.Example, ui.Count(s.ExampleKeys, "key")))
}

func unreadNote(s envfiles.Set) string {
	return strings.Join(s.Unread, ", ") + " is a pipe, as 1Password Environments makes: it is never opened, so its keys are not compared"
}

// alerts are what needs acting on first: files whose secrets may have leaked.
func alerts(s envfiles.Set, now time.Time) []string {
	var alerts []string
	if s.Err != nil {
		alerts = append(alerts, ui.Danger.Render("Could not read: "+s.Err.Error()))
	}
	for _, file := range s.Tracked {
		alerts = append(alerts, ui.Danger.Render(file+" is tracked by git")+
			ui.Muted.Render(": untrack it with git rm --cached "+file+", then rotate its secrets"))
	}
	for _, c := range s.Committed {
		alerts = append(alerts, ui.Danger.Render(c.File+" is in git history")+
			ui.Muted.Render(fmt.Sprintf(": added in %s %s and deleted since; rotate any real secret it held", c.Hash, ui.Ago(now, c.At))))
	}
	return alerts
}

// body lays out what your files lack beside what the example lacks, or one below the other when narrow.
func body(s envfiles.Set, width, height int) []string {
	if s.Err != nil {
		return nil
	}
	yours := yourSections(s)
	example := exampleSections(s)
	if len(yours)+len(example) == 0 {
		return []string{ui.FitLine(allGood(s), width)}
	}
	if width+detailFrameWidth < twoColumnWidth || len(yours) == 0 || len(example) == 0 {
		return ui.FitSections(append(yours, example...), width, height)
	}
	separatorWidth := lipgloss.Width(columnSeparator)
	leftWidth := (width - separatorWidth) / 2
	rightWidth := width - separatorWidth - leftWidth
	return ui.SideBySide(ui.FitSections(yours, leftWidth, height), ui.FitSections(example, rightWidth, height), leftWidth, columnSeparator)
}

func allGood(s envfiles.Set) string {
	switch {
	case s.Example != "" && len(s.Locals) == 0:
		return ui.Muted.Render("Not set up here yet: copy " + s.Example + " to " + s.Base() + " and fill it in")
	case s.Example == "" && len(s.Locals) > 0:
		return ui.Muted.Render("Add an example listing these keys, so others know what to set")
	case s.Example == "", len(s.Unread) > 0:
		return ""
	}
	return ui.Success.Render("Every key " + s.Example + " lists is set")
}

// yourSections are what your local files lack: keys missing, and keys with no value.
func yourSections(s envfiles.Set) []ui.Section {
	var sections []ui.Section
	if len(s.Missing) > 0 {
		section := ui.Section{Title: ui.Heading.Render("MISSING") + "  " + strconv.Itoa(len(s.Missing)) + ui.Muted.Render(noteJoiner+"a adds them")}
		for _, key := range s.Missing {
			section.Items = append(section.Items, ui.Warning.Render(key))
		}
		sections = append(sections, section)
	}
	if len(s.Empty) > 0 {
		sections = append(sections, keysInFiles("EMPTY", s.Empty, len(s.Locals) > 1))
	}
	return sections
}

// exampleSections are what the example lacks: keys your files have, and keys the code reads.
func exampleSections(s envfiles.Set) []ui.Section {
	var sections []ui.Section
	if len(s.Extra) > 0 {
		sections = append(sections, keysInFiles("NOT IN EXAMPLE", s.Extra, len(s.Locals) > 1))
	}
	if len(s.InCode) > 0 {
		section := ui.Section{Title: ui.Heading.Render("USED IN CODE") + "  " + strconv.Itoa(len(s.InCode)) + ui.Muted.Render(noteJoiner+"not in the example")}
		for _, ref := range s.InCode {
			section.Items = append(section.Items, accent.Render(ref.Key)+ui.Muted.Render(fmt.Sprintf("  %s:%d", ref.File, ref.Line)))
		}
		sections = append(sections, section)
	}
	return sections
}

// keysInFiles lists keys, naming each one's file when there are several to tell apart.
func keysInFiles(title string, keys []envfiles.KeyInFile, showFile bool) ui.Section {
	section := ui.Section{Title: ui.Heading.Render(title) + "  " + strconv.Itoa(len(keys))}
	for _, k := range keys {
		item := k.Key
		if showFile {
			item += ui.Muted.Render("  " + k.File)
		}
		section.Items = append(section.Items, item)
	}
	return section
}

func padLines(lines []string) string {
	for len(lines) < detailLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
