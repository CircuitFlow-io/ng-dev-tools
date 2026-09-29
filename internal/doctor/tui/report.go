package tui

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/doctor"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

const (
	rowIndent  = "  "
	iconWidth  = 2
	nameGutter = 2
	fixLabel   = "fix: "
)

var fixStyle = lipgloss.NewStyle().Foreground(ui.ColorAccent2)

// statusLook is how a status is drawn.
type statusLook struct {
	icon    string
	style   lipgloss.Style
	counted string
}

var looks = map[doctor.Status]statusLook{
	doctor.StatusFail: {"✘", ui.Danger, "failed"},
	doctor.StatusWarn: {"!", ui.Warning.Bold(true), "warning"},
	doctor.StatusPass: {"✔", ui.Success, "passed"},
	doctor.StatusSkip: {"–", ui.Muted, "skipped"},
}

// ReportOptions tune the report.
type ReportOptions struct {
	// ProblemsOnly hides checks that passed or were skipped.
	ProblemsOnly bool
	Elapsed      time.Duration
}

// Report renders the outcomes grouped by section, with fixes under each problem and a tally at the end.
func Report(outcomes []doctor.Outcome, opts ReportOptions) string {
	shown := outcomes
	if opts.ProblemsOnly {
		shown = slices.DeleteFunc(slices.Clone(outcomes), func(o doctor.Outcome) bool { return !o.Result.IsProblem() })
	}
	var sections []string
	if len(shown) == 0 {
		sections = append(sections, ui.Success.Render("✔ No problems found."))
	}
	nameWidth := longestName(shown) + nameGutter
	for _, group := range groupsOf(shown) {
		sections = append(sections, renderGroup(group, shown, nameWidth))
	}
	sections = append(sections, tally(outcomes, opts.Elapsed))
	return strings.Join(sections, "\n\n")
}

func groupsOf(outcomes []doctor.Outcome) []doctor.Group {
	var groups []doctor.Group
	for _, o := range outcomes {
		if !slices.Contains(groups, o.Check.Group) {
			groups = append(groups, o.Check.Group)
		}
	}
	return groups
}

func longestName(outcomes []doctor.Outcome) int {
	var longest int
	for _, o := range outcomes {
		longest = max(longest, lipgloss.Width(o.Check.Name))
	}
	return longest
}

func renderGroup(group doctor.Group, outcomes []doctor.Outcome, nameWidth int) string {
	lines := []string{ui.Heading.Render(group.Title())}
	for _, o := range outcomes {
		if o.Check.Group == group {
			lines = append(lines, renderOutcome(o, nameWidth)...)
		}
	}
	return strings.Join(lines, "\n")
}

func renderOutcome(o doctor.Outcome, nameWidth int) []string {
	r := o.Result
	look := looks[r.Status]
	name := ui.PadRight(o.Check.Name, nameWidth)
	summary := ui.Muted.Render(r.Summary)
	if r.IsProblem() {
		name = ui.Bold.Render(name)
		summary = look.style.UnsetBold().Render(r.Summary)
	}
	lines := []string{rowIndent + look.style.Render(look.icon) + " " + name + summary}

	indent := strings.Repeat(" ", len(rowIndent)+iconWidth+nameWidth)
	for _, detail := range r.Details {
		lines = append(lines, indent+ui.Muted.Render(detail))
	}
	if r.Fix != "" && r.IsProblem() {
		lines = append(lines, indent+ui.Muted.Render(fixLabel)+fixStyle.Render(r.Fix))
	}
	return lines
}

func tally(outcomes []doctor.Outcome, elapsed time.Duration) string {
	var parts []string
	for _, status := range []doctor.Status{doctor.StatusFail, doctor.StatusWarn, doctor.StatusPass, doctor.StatusSkip} {
		n := doctor.Count(outcomes, status)
		if n == 0 {
			continue
		}
		look := looks[status]
		label := fmt.Sprintf("%d %s", n, look.counted)
		if status == doctor.StatusWarn {
			label = ui.Count(n, look.counted)
		}
		parts = append(parts, look.style.Render(look.icon+" "+label))
	}
	separator := ui.Muted.Render(" · ")
	return strings.Join(parts, separator) + ui.Muted.Render(fmt.Sprintf("   in %.1fs", elapsed.Seconds()))
}
