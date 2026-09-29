package tui

import (
	"fmt"
	"strings"

	"github.com/nasserghiasi/ng-dev-tools/internal/ports"
	"github.com/nasserghiasi/ng-dev-tools/internal/ui"
)

func (m Model) summaryView() string {
	switch {
	case m.err != nil:
		return ui.Danger.Render("✘ " + m.err.Error())
	case m.results != nil:
		return Results(m.results)
	case m.aborted:
		return ui.Muted.Render("Cancelled. Nothing was stopped.")
	}
	message := ui.Success.Render("✔ Nothing is listening on a TCP port.")
	if m.hidden > 0 {
		message += ui.Muted.Render(fmt.Sprintf(" (%s hidden, --all shows them)", ui.Count(m.hidden, "macOS system process")))
	}
	return message
}

// Results summarises what happened to each process.
func Results(results []ports.StopResult) string {
	var stopped, failed []string
	for _, r := range results {
		if r.Err != nil {
			failed = append(failed, "  • "+r.Process.Label()+": "+ui.Muted.Render(r.Err.Error()))
			continue
		}
		stopped = append(stopped, "  • "+r.Process.Label()+ui.Muted.Render(outcomeNote(r.Outcome)))
	}

	var sections []string
	if len(stopped) > 0 {
		heading := ui.Success.Bold(true).Render("✔ Stopped " + ui.Count(len(stopped), "process"))
		sections = append(sections, heading+"\n"+strings.Join(stopped, "\n"))
	}
	if len(failed) > 0 {
		heading := ui.Danger.Render("✘ Could not stop " + ui.Count(len(failed), "process"))
		sections = append(sections, heading+"\n"+strings.Join(failed, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func outcomeNote(o ports.Outcome) string {
	switch o {
	case ports.Killed:
		return "  (killed with SIGKILL)"
	case ports.AlreadyExited:
		return "  (had already exited)"
	default:
		return ""
	}
}
