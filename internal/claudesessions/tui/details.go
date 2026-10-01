package tui

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// detailLines is the fixed height of the details box's content, so the table does not jump:
	// two header lines, the facts beside the pull requests, and the prompts.
	detailLines      = 9
	headerLines      = 2
	promptLines      = 2
	bodyLines        = detailLines - headerLines - promptLines
	detailFrameWidth = 4
	// twoColumnWidth is the terminal width from which the pull requests sit beside the facts.
	twoColumnWidth  = 100
	columnSeparator = " │ "
	joiner          = " · "
	untitled        = "Untitled"
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
	usage   claudesessions.Usage
	prs     prLookup
	missing bool
	now     time.Time
	home    string
}

// details describes the session under the cursor. The header names it, says what Claude is doing
// with it and where it ran; below, its facts sit beside its pull requests, then come its first and
// last prompts, or where the search matched.
func details(in detailsInput, ok bool, width int) string {
	inner := width - detailFrameWidth
	if !ok {
		return detailBox.Width(width).Render(padLines([]string{ui.Muted.Render("Nothing to resume")}))
	}
	s := in.result.Session
	lines := []string{
		ui.Spread(titleText(s), statusStyle(in.live).Render(liveSummary(in.live, in.now)), inner),
		ui.Spread(folderLine(s, in.missing, in.home), ui.Muted.Render(s.ID), inner),
	}
	lines = append(lines, body(in, inner)...)
	lines = append(lines, prompts(in.result)...)
	for i, line := range lines {
		lines[i] = ui.FitLine(line, inner)
	}
	return detailBox.Width(width).Render(padLines(lines))
}

func titleText(s claudesessions.Session) string {
	if s.Title == "" {
		return ui.Bold.Render(untitled)
	}
	return ui.Bold.Render(s.Title)
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

// facts are the session's labelled facts, one line each.
type facts struct {
	when, size, models, tokens, cache string
}

func sessionFacts(in detailsInput) facts {
	s := in.result.Session
	f := facts{
		when:   labelled("When", "started "+ui.Ago(in.now, s.Started)+ui.Muted.Render(joiner)+"active "+ui.Ago(in.now, s.LastActive)),
		size:   labelled("Size", ui.Count(s.Prompts, "prompt")+ui.Muted.Render(joiner)+ui.Bytes(s.Size)),
		models: labelled("Models", ui.Muted.Render("none")),
		tokens: labelled("Tokens", ui.Muted.Render("not recorded")),
	}
	var models []string
	for _, m := range s.Models {
		models = append(models, claudesessions.ModelName(m.ID)+ui.Muted.Render(" ("+ui.Count(m.Replies, "reply")+")"))
	}
	if len(models) > 0 {
		f.models = labelled("Models", strings.Join(models, ui.Muted.Render(", ")))
	}
	if !in.usage.IsZero() {
		f.tokens = labelled("Tokens", tokens(in.usage))
		f.cache = labelled("Cache", cache(in.usage))
	}
	return f
}

// body puts the facts beside the pull requests, or, when narrow, the main facts above a line
// listing the pull requests.
func body(in detailsInput, width int) []string {
	f := sessionFacts(in)
	urls := in.result.Session.PRs
	if width+detailFrameWidth < twoColumnWidth {
		return []string{f.when, f.models, f.tokens, f.cache, labelled("PRs", prInline(urls, in.prs))}
	}
	separatorWidth := lipgloss.Width(columnSeparator)
	leftWidth := (width - separatorWidth) / 2
	left := []string{f.when, f.size, f.models, f.tokens, f.cache}
	for i, line := range left {
		left[i] = ui.FitLine(line, leftWidth)
	}
	right := ui.FitSections([]ui.Section{prSection(urls, in.prs)}, width-separatorWidth-leftWidth, bodyLines)
	return ui.SideBySide(left, right, leftWidth, columnSeparator)
}

// prompts are the first prompt and the last, or where the search matched.
func prompts(r claudesessions.Result) []string {
	s := r.Session
	lines := []string{labelled("First", s.FirstPrompt)}
	if !r.Snippet.IsZero() {
		return append(lines, snippetLine(r.Snippet))
	}
	if s.Prompts > 1 {
		lines = append(lines, labelled("Last", s.LastPrompt))
	}
	return lines
}

// tokens leads with the context size, the one that says how close the session is to compacting.
func tokens(u claudesessions.Usage) string {
	return accent.Render(ui.Compact(u.Context)+" context") +
		ui.Muted.Render(joiner+ui.Compact(u.Output)+" out"+joiner+ui.Compact(u.Input)+" in")
}

func cache(u claudesessions.Usage) string {
	return ui.Compact(u.CacheRead) + " read" + ui.Muted.Render(joiner+ui.Compact(u.CacheWrite)+" written")
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
