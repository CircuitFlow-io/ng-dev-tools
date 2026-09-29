package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// detailLines is the fixed height of the details box's content, so the table does not jump.
	detailLines      = 8
	detailFrameWidth = 4
	// twoColumnWidth is the terminal width from which changes and commits sit side by side.
	twoColumnWidth  = 100
	columnSeparator = " │ "
	ellipsis        = "…"
	// minItemsForMoreLine is how many items a cut-short section needs before one of them gives way
	// to an "… n more" line.
	minItemsForMoreLine = 2
)

var (
	detailBox    = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ui.ColorAccent).Padding(0, 1)
	sectionLabel = ui.Heading
)

// String renders the cell's spans without padding.
func (c cell) String() string {
	var b strings.Builder
	for _, s := range c {
		b.WriteString(s.style.Render(s.text))
	}
	return b.String()
}

// section is a titled list in the details box, such as the stashes.
type section struct {
	title string
	items []string
}

// details describes the repository under the cursor.
func details(r gitstatus.Repo, ok bool, s syncState, now time.Time, width int) string {
	inner := width - detailFrameWidth
	if !ok {
		return detailBox.Width(width).Render(padLines([]string{ui.Muted.Render("No repository selected")}))
	}
	lines := []string{fit(headerLine(r, s, now), inner)}
	for _, alert := range alerts(r, s) {
		lines = append(lines, fit(alert, inner))
	}
	if room := detailLines - len(lines); room > 0 {
		lines = append(lines, body(r, now, inner, room)...)
	}
	return detailBox.Width(width).Render(padLines(lines[:min(len(lines), detailLines)]))
}

func headerLine(r gitstatus.Repo, s syncState, now time.Time) string {
	line := ui.Bold.Render(r.Name) + "  " + branchStyle.Render(r.Branch)
	facts := []string{upstreamFact(r)}
	if fetched := fetchFact(r, s, now); fetched != "" {
		facts = append(facts, fetched)
	}
	return line + ui.Muted.Render(noteJoiner+strings.Join(facts, noteJoiner))
}

func upstreamFact(r gitstatus.Repo) string {
	switch {
	case r.Err != nil:
		return "unreadable"
	case r.Unborn:
		return "no commits yet"
	case !r.HasRemote:
		return "no remote"
	case r.Detached:
		return "detached HEAD"
	case r.UpstreamGone:
		return upstreamLabel(r) + " was deleted on the remote"
	case r.NotPushed():
		return "not pushed yet"
	}
	var parts []string
	if r.Ahead > 0 {
		parts = append(parts, fmt.Sprintf("%d ahead", r.Ahead))
	}
	if r.Behind > 0 {
		parts = append(parts, fmt.Sprintf("%d behind", r.Behind))
	}
	if len(parts) == 0 {
		return "in sync with " + upstreamLabel(r)
	}
	return strings.Join(parts, ", ") + " of " + upstreamLabel(r)
}

// upstreamLabel names the upstream by its remote alone when the branch names match, as they
// usually do: "origin" rather than "origin/feat/long-branch-name".
func upstreamLabel(r gitstatus.Repo) string {
	if remote, ok := strings.CutSuffix(r.Upstream, "/"+r.Branch); ok {
		return remote
	}
	return r.Upstream
}

func fetchFact(r gitstatus.Repo, s syncState, now time.Time) string {
	switch {
	case !r.HasRemote:
		return ""
	case s.fetching:
		return "fetching…"
	case r.FetchedAt.IsZero():
		return "never fetched"
	}
	return "fetched " + ui.Ago(now, r.FetchedAt)
}

// alerts are what needs acting on before anything else, most serious first.
func alerts(r gitstatus.Repo, s syncState) []string {
	var alerts []string
	if r.Err != nil {
		alerts = append(alerts, ui.Danger.Render("Could not read the repository: "+firstLine(r.Err.Error())))
	}
	if r.Operation != gitstatus.NoOperation {
		alerts = append(alerts, ui.Danger.Render(r.Operation.Name()+" in progress")+ui.Muted.Render(": finish with "+r.Operation.Finish()))
	}
	if s.fetchErr != "" {
		alerts = append(alerts, ui.Warning.Render("Fetch failed: "+s.fetchErr))
	}
	return alerts
}

// body lays out the changes beside the commits, stashes and branches, or below them when narrow.
func body(r gitstatus.Repo, now time.Time, width, height int) []string {
	if r.Err != nil {
		return nil
	}
	changes := []section{changesSection(r, width)}
	others := otherSections(r, now)
	if width+detailFrameWidth < twoColumnWidth {
		return fitSections(append(changes, others...), width, height)
	}
	separatorWidth := lipgloss.Width(columnSeparator)
	leftWidth := (width - separatorWidth) / 2
	rightWidth := width - separatorWidth - leftWidth
	left := fitSections(changes, leftWidth, height)
	right := fitSections(others, rightWidth, height)
	if len(others) == 0 {
		right = []string{fit(ui.Muted.Render(nothingElse(r)), rightWidth)}
	}
	lines := make([]string, max(len(left), len(right)))
	for i := range lines {
		l, rt := "", ""
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			rt = right[i]
		}
		lines[i] = ui.PadRight(l, leftWidth) + ui.Muted.Render(columnSeparator) + rt
	}
	return lines
}

func nothingElse(r gitstatus.Repo) string {
	switch {
	case r.Unborn:
		return "No commits yet"
	case !r.HasRemote:
		return "No remote, so nothing here is backed up"
	}
	return "Nothing unpushed, no stashes or other branches"
}

func changesSection(r gitstatus.Repo, width int) section {
	c := r.Changes()
	if !c.Any() {
		return section{title: sectionLabel.Render("CHANGES") + "  " + ui.Success.Render("working tree clean")}
	}
	var counts []string
	for _, count := range []struct {
		n    int
		noun string
	}{{c.Conflicted, "conflicted"}, {c.Staged, "staged"}, {c.Modified, "modified"}, {c.Untracked, "untracked"}} {
		if count.n > 0 {
			counts = append(counts, fmt.Sprintf("%d %s", count.n, count.noun))
		}
	}
	s := section{title: sectionLabel.Render("CHANGES") + "  " + strings.Join(counts, noteJoiner)}
	for _, f := range r.Files {
		s.items = append(s.items, fileLine(f, width))
	}
	return s
}

func fileLine(f gitstatus.File, width int) string {
	path := f.Path
	if f.From != "" {
		path = f.From + " → " + f.Path
	}
	code := f.Code()
	return fileCode(f, code) + " " + ui.TruncatePath(path, width-lipgloss.Width(code)-1)
}

func fileCode(f gitstatus.File, code string) string {
	switch f.Kind {
	case gitstatus.Untracked:
		return ui.Muted.Render(code)
	case gitstatus.Unmerged:
		return ui.Danger.Render(code)
	}
	return ui.Success.Render(code[:1]) + ui.Warning.Render(code[1:])
}

func otherSections(r gitstatus.Repo, now time.Time) []section {
	var sections []section
	if r.Unpushed > 0 {
		s := section{title: sectionLabel.Render("NOT PUSHED") + "  " + ui.Count(r.Unpushed, "commit")}
		for _, c := range r.UnpushedCommits {
			s.items = append(s.items, accent.Render(c.Hash)+" "+c.Subject)
		}
		sections = append(sections, s)
	}
	if len(r.Stashes) > 0 {
		s := section{title: sectionLabel.Render("STASHES") + "  " + fmt.Sprint(len(r.Stashes))}
		for _, stash := range r.Stashes {
			s.items = append(s.items, ui.Muted.Render(stash.Ref+"  "+ui.Ago(now, stash.At)+"  ")+stash.Message)
		}
		sections = append(sections, s)
	}
	if len(r.Branches) > 0 {
		s := section{title: sectionLabel.Render("BRANCHES") + "  " + fmt.Sprint(len(r.Branches))}
		if len(r.Branches) > 1 {
			s.title += ui.Muted.Render(noteJoiner + branchKinds(r))
		}
		for _, b := range r.Branches {
			s.items = append(s.items, branchCell(b).String()+ui.Muted.Render("  "+ui.Ago(now, b.At)))
		}
		sections = append(sections, s)
	}
	return sections
}

// fitSections shows each section's title and shares the lines left among their items in turn,
// so a long list of files does not hide the stashes.
func fitSections(sections []section, width, height int) []string {
	for len(sections) > height {
		sections = sections[:len(sections)-1]
	}
	shown := make([]int, len(sections))
	for room, more := height-len(sections), true; room > 0 && more; {
		more = false
		for i, s := range sections {
			if room > 0 && shown[i] < len(s.items) {
				shown[i]++
				room--
				more = true
			}
		}
	}
	var lines []string
	for i, s := range sections {
		lines = append(lines, fit(s.title, width))
		for _, item := range visibleItems(s.items, shown[i]) {
			lines = append(lines, fit(item, width))
		}
	}
	return lines
}

// visibleItems is the first n items, the last replaced by "… k more" when some are left out.
func visibleItems(items []string, n int) []string {
	if n >= len(items) || n < minItemsForMoreLine {
		return items[:n]
	}
	visible := append([]string(nil), items[:n-1]...)
	return append(visible, ui.Muted.Render(fmt.Sprintf("%s %d more", ellipsis, len(items)-n+1)))
}

// fit cuts a styled line to width.
func fit(line string, width int) string {
	return ansi.Truncate(line, max(width, 0), ellipsis)
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	return line
}

func padLines(lines []string) string {
	for len(lines) < detailLines {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
