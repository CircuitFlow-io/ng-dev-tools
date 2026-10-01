package tui

import (
	"slices"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	prsHeading = "PULL REQUESTS"
	// stateWidth lines up the titles after the longest state, "merged" or "closed".
	stateWidth  = len("merged") + 2
	unknownMark = "·"
)

// prLookup is where looking up the sessions' pull requests on GitHub stands.
type prLookup struct {
	pending bool
	found   map[string]pulls.Summary
	err     error
}

// prState is how a pull request's state is shown.
type prState struct {
	mark, word string
	style      lipgloss.Style
}

var (
	merged = prState{"✓", "merged", lipgloss.NewStyle().Foreground(ui.ColorAccent)}
	closed = prState{"✕", "closed", ui.Danger}
	draft  = prState{"◌", "draft", ui.Muted}
	open   = prState{"○", "open", ui.Success}
	// stateOrder is the order the summary counts them in, what still needs you first.
	stateOrder = []prState{open, draft, merged, closed}
)

func stateOf(pr pulls.Summary) prState {
	switch {
	case pr.State == pulls.StateMerged:
		return merged
	case pr.State == pulls.StateClosed:
		return closed
	case pr.Draft:
		return draft
	default:
		return open
	}
}

// prSection lists the session's pull requests, the latest first, with where each stands, after
// why they could not be looked up when that failed.
func prSection(urls []string, lookup prLookup) ui.Section {
	section := ui.Section{Title: ui.Heading.Render(prsHeading) + "  " + prSummary(urls, lookup)}
	if lookup.err != nil && len(urls) > 0 {
		section.Items = append(section.Items, ui.Muted.Render(lookup.err.Error()))
	}
	for _, url := range slices.Backward(urls) {
		section.Items = append(section.Items, prLine(url, lookup))
	}
	return section
}

// prSummary counts the pull requests by state, or says why it cannot.
func prSummary(urls []string, lookup prLookup) string {
	switch {
	case len(urls) == 0:
		return ui.Muted.Render("none")
	case lookup.err != nil:
		return ui.Warning.Render("could not look up on GitHub")
	case lookup.pending:
		return ui.Muted.Render("looking up on GitHub…")
	case lookup.found == nil:
		return ui.Muted.Render(strconv.Itoa(len(urls)))
	}
	counts := map[string]int{}
	for _, url := range urls {
		if pr, ok := lookup.found[url]; ok {
			counts[stateOf(pr).word]++
		}
	}
	var parts []string
	for _, state := range stateOrder {
		if n := counts[state.word]; n > 0 {
			parts = append(parts, state.style.Render(strconv.Itoa(n)+" "+state.word))
		}
	}
	return strings.Join(parts, ui.Muted.Render(joiner))
}

// prLine is a pull request's state, number and title, or its address while it is not known.
func prLine(url string, lookup prLookup) string {
	pr, ok := lookup.found[url]
	if !ok {
		return ui.Muted.Render(unknownMark + " " + prRef(url))
	}
	state := stateOf(pr)
	return state.style.Render(state.mark+" "+ui.PadRight(state.word, stateWidth)) + "#" + strconv.Itoa(pr.Number) + "  " + ui.RenderTickets(pr.Title, plain)
}

// prInline lists the pull requests on one line, the latest first, for a narrow box.
func prInline(urls []string, lookup prLookup) string {
	if len(urls) == 0 || lookup.err != nil || lookup.pending {
		return prSummary(urls, lookup)
	}
	var parts []string
	for _, url := range slices.Backward(urls) {
		pr, ok := lookup.found[url]
		if !ok {
			parts = append(parts, ui.Muted.Render(prRef(url)))
			continue
		}
		state := stateOf(pr)
		parts = append(parts, "#"+strconv.Itoa(pr.Number)+" "+state.style.Render(state.word))
	}
	return strings.Join(parts, ui.Muted.Render(joiner))
}

// prRef names a pull request by its address: "owner/repo#12", or the URL when it is not GitHub's.
func prRef(url string) string {
	repo, number, ok := pulls.ParseURL(url)
	if !ok {
		return url
	}
	return repo + "#" + strconv.Itoa(number)
}
