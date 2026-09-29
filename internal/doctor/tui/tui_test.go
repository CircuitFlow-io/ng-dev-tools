package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/doctor"
)

func outcome(name string, group doctor.Group, result doctor.Result) doctor.Outcome {
	return doctor.Outcome{Check: doctor.Check{Name: name, Group: group}, Result: result}
}

var sampleOutcomes = []doctor.Outcome{
	outcome("nvm", doctor.GroupNode, doctor.Result{Status: doctor.StatusPass, Summary: "0.40.3"}),
	outcome("Node.js", doctor.GroupNode, doctor.Result{
		Status: doctor.StatusFail, Summary: "22.13.1, the latest LTS is 24.9.0",
		Details: []string{"nvm default: 22.13"}, Fix: "nvm install --lts",
	}),
	outcome("Firewall", doctor.GroupSystem, doctor.Result{Status: doctor.StatusWarn, Summary: "the firewall is off", Fix: "System Settings"}),
	outcome("Battery", doctor.GroupSystem, doctor.Result{Status: doctor.StatusSkip, Summary: "no battery"}),
}

func plain(s string) string {
	return ansi.Strip(s)
}

func TestReportGroupsChecksAndShowsFixes(t *testing.T) {
	report := plain(Report(sampleOutcomes, ReportOptions{Elapsed: 4200 * time.Millisecond}))

	for _, want := range []string{
		"Node.js & JavaScript", "System",
		"✔ nvm", "✘ Node.js", "! Firewall", "– Battery",
		"nvm default: 22.13", "fix: nvm install --lts",
		"✘ 1 failed · ! 1 warning · ✔ 1 passed · – 1 skipped", "in 4.2s",
	} {
		if !strings.Contains(report, want) {
			t.Errorf("report is missing %q:\n%s", want, report)
		}
	}
	if strings.Index(report, "Node.js & JavaScript") > strings.Index(report, "System") {
		t.Error("groups are out of catalog order")
	}
}

func TestReportProblemsOnly(t *testing.T) {
	report := plain(Report(sampleOutcomes, ReportOptions{ProblemsOnly: true}))

	if strings.Contains(report, "nvm  ") || strings.Contains(report, "Battery") {
		t.Errorf("passing or skipped checks shown:\n%s", report)
	}
	if !strings.Contains(report, "Firewall") {
		t.Errorf("problem hidden:\n%s", report)
	}
}

func TestReportWithoutProblems(t *testing.T) {
	report := plain(Report(sampleOutcomes[:1], ReportOptions{ProblemsOnly: true}))

	if !strings.Contains(report, "No problems found") {
		t.Errorf("report = %q", report)
	}
}

func newTestModel() Model {
	checks := []doctor.Check{{Name: "a"}, {Name: "b"}}
	return New(context.Background(), doctor.Doctor{Checks: checks})
}

func TestModelTracksProgressAndFinishes(t *testing.T) {
	m := newTestModel()

	next, _ := m.Update(progressMsg(doctor.Progress{Done: 1, Total: 2, Current: "b"}))
	m = next.(Model)
	if view := plain(m.View().Content); !strings.Contains(view, "1/2 checks") {
		t.Errorf("view = %q", view)
	}

	next, cmd := m.Update(doneMsg(sampleOutcomes))
	m = next.(Model)
	if cmd == nil || len(m.Outcomes()) != len(sampleOutcomes) || m.Cancelled() {
		t.Errorf("finished model: outcomes %d, cancelled %v", len(m.Outcomes()), m.Cancelled())
	}
}

func TestModelCancels(t *testing.T) {
	next, cmd := newTestModel().Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	m := next.(Model)

	if !m.Cancelled() || cmd == nil {
		t.Error("q did not cancel")
	}
	if m.ctx.Err() == nil {
		t.Error("checks were not told to stop")
	}
}
