package tui

import (
	"fmt"
	"strings"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	// logChromeLines is the space around the log: top margin, title, blank line and help.
	logChromeLines = 5
	errorMarker    = "##[error]"
	warningMarker  = "##[warning]"
	groupMarker    = "##[group]"
	endGroupMarker = "##[endgroup]"
	logHelp        = "↑/↓ pgup/pgdn scroll · o open in browser · esc back"
	nextCheckHelp  = " · tab next failed check"
)

// logView shows the log of a pull request's failed checks, one at a time, scrolled to the end
// where the failure usually is.
type logView struct {
	pr       pulls.PR
	checks   []pulls.Check
	index    int
	loading  bool
	err      error
	viewport viewport.Model
}

func newLogView(p pulls.PR, width, height int) logView {
	v := logView{pr: p, checks: p.FailingChecks(), loading: true, viewport: viewport.New()}
	v.viewport.SoftWrap = true
	v.resize(width, height)
	return v
}

func (v *logView) resize(width, height int) {
	v.viewport.SetWidth(width)
	v.viewport.SetHeight(max(height-logChromeLines, 1))
}

func (v logView) check() pulls.Check {
	return v.checks[v.index]
}

// next moves to the following failed check, wrapping around, and reports whether there was one.
func (v *logView) next() bool {
	if len(v.checks) < 2 {
		return false
	}
	v.index = (v.index + 1) % len(v.checks)
	v.loading, v.err = true, nil
	v.viewport.SetContent("")
	return true
}

func (v *logView) show(lines []pulls.LogLine, err error) {
	v.loading, v.err = false, err
	v.viewport.SetContent(renderLog(lines))
	v.viewport.GotoBottom()
}

func (v logView) update(msg tea.Msg) (logView, tea.Cmd) {
	var cmd tea.Cmd
	v.viewport, cmd = v.viewport.Update(msg)
	return v, cmd
}

func (v logView) view(spinner string) string {
	check := v.check()
	title := ui.Title.Render("Log of "+check.Name) + ui.Muted.Render(noteJoiner+v.pr.Ref())
	if len(v.checks) > 1 {
		title += ui.Muted.Render(fmt.Sprintf("%sfailed check %d of %d", noteJoiner, v.index+1, len(v.checks)))
	}
	help := logHelp
	if len(v.checks) > 1 {
		help += nextCheckHelp
	}
	var content string
	switch {
	case v.loading:
		content = spinner + " " + ui.Muted.Render("Reading the log…")
	case v.err != nil:
		content = ui.Warning.Render(v.err.Error()) + "\n" + ui.Muted.Render("Press o to open the job in your browser.")
	default:
		content = v.viewport.View()
	}
	return strings.Join([]string{title, "", content, ui.Help.Render(help)}, "\n")
}

// renderLog puts a heading before each step and colours GitHub's error and warning lines.
func renderLog(lines []pulls.LogLine) string {
	var out []string
	step := ""
	for _, l := range lines {
		if l.Step != step {
			step = l.Step
			if len(out) > 0 {
				out = append(out, "")
			}
			out = append(out, ui.Heading.Render("▸ "+step))
		}
		if rendered, ok := renderLogText(l.Text); ok {
			out = append(out, rendered)
		}
	}
	if len(out) == 0 {
		return ui.Muted.Render("The failed steps left no output.")
	}
	return strings.Join(out, "\n")
}

func renderLogText(text string) (string, bool) {
	switch {
	case strings.HasPrefix(text, endGroupMarker):
		return "", false
	case strings.HasPrefix(text, errorMarker):
		return ui.Danger.Render(strings.TrimPrefix(text, errorMarker)), true
	case strings.HasPrefix(text, warningMarker):
		return ui.Warning.Render(strings.TrimPrefix(text, warningMarker)), true
	case strings.HasPrefix(text, groupMarker):
		return ui.Muted.Render(strings.TrimPrefix(text, groupMarker)), true
	}
	return text, true
}
