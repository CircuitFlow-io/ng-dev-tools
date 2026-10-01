package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// detailLines is the fixed height of the details box's content, so the table does not jump.
	detailLines      = 6
	detailFrameWidth = 4
	joiner           = " · "
	untitled         = "Untitled"
	// labelWidth lines up the text after the longest label, "Claude".
	labelWidth = len("Claude") + 2
)

var (
	detailBox  = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.ColorAccent).Padding(0, 1)
	matchStyle = lipgloss.NewStyle().Foreground(ui.ColorAccent2).Bold(true).Underline(true)
)

// detailsInput is what the details box describes.
type detailsInput struct {
	result  claudesessions.Result
	live    claudesessions.Live
	missing bool
	now     time.Time
	home    string
}

// details describes the session under the cursor: its title and what Claude is doing with it, folder, when and how long it ran,
// its models and pull requests, and its first and last prompts, or where the search matched.
func details(in detailsInput, ok bool, width int) string {
	inner := width - detailFrameWidth
	if !ok {
		return detailBox.Width(width).Render(padLines([]string{ui.Muted.Render("Nothing to resume")}))
	}
	s := in.result.Session
	lines := []string{
		titleLine(s, in.live, in.now),
		folderLine(s, in.missing, in.home),
		factsLine(s, in.now),
	}
	if len(s.PRs) > 0 {
		lines = append(lines, labelled("PRs", accent.Render(strings.Join(s.PRs, "  "))))
	}
	lines = append(lines, labelled("First", s.FirstPrompt))
	if snippet := in.result.Snippet; !snippet.IsZero() {
		lines = append(lines, snippetLine(snippet))
	} else if s.Prompts > 1 {
		lines = append(lines, labelled("Last", s.LastPrompt))
	}
	for i, line := range lines {
		lines[i] = ui.FitLine(line, inner)
	}
	return detailBox.Width(width).Render(padLines(lines))
}

func titleLine(s claudesessions.Session, live claudesessions.Live, now time.Time) string {
	title := s.Title
	if title == "" {
		title = untitled
	}
	line := ui.Bold.Render(title) + ui.Muted.Render("  "+s.ID)
	if summary := liveSummary(live, now); summary != "" {
		line += "  " + statusStyle(live).Render(summary)
	}
	return line
}

func folderLine(s claudesessions.Session, missing bool, home string) string {
	line := accent.Render(ui.TildePath(s.Dir, home))
	if missing {
		line += ui.Warning.Render("  no longer exists, so it cannot be resumed")
	}
	if len(s.Branches) > 0 {
		line += ui.Muted.Render(joiner+"on ") + strings.Join(s.Branches, ui.Muted.Render(", "))
	}
	return line
}

func factsLine(s claudesessions.Session, now time.Time) string {
	facts := []string{
		"started " + ui.Ago(now, s.Started),
		"active " + ui.Ago(now, s.LastActive),
		ui.Count(s.Prompts, "prompt"),
		ui.Bytes(s.Size),
	}
	var models []string
	for _, m := range s.Models {
		models = append(models, fmt.Sprintf("%s (%s)", claudesessions.ModelName(m.ID), ui.Count(m.Replies, "reply")))
	}
	if len(models) > 0 {
		facts = append(facts, strings.Join(models, ", "))
	}
	return ui.Muted.Render(strings.Join(facts, joiner))
}

func labelled(label, text string) string {
	return ui.Muted.Render(ui.PadRight(label, labelWidth)) + text
}

// snippetLine shows where the search matched, labelled with who wrote it.
func snippetLine(s claudesessions.Snippet) string {
	who := "Claude"
	if s.Yours {
		who = "You"
	}
	return labelled(who, s.Before+matchStyle.Render(s.Match)+s.After)
}

func padLines(lines []string) string {
	lines = lines[:min(len(lines), detailLines)]
	for len(lines) < detailLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
