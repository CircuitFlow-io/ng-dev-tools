package tui

import (
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos/macostest"
	"github.com/nasserghiasi/ng-dev-tools/internal/ports"
)

func sampleProcesses() []ports.Process {
	started := time.Now().Add(-time.Hour)
	return []ports.Process{
		{PID: 501, Name: "node", Ports: []int{3000}, Addresses: []string{"127.0.0.1:3000"}, StartedAt: started},
		{PID: 663, Name: "ControlCenter", Executable: "/System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter", Ports: []int{5000}},
		{PID: 77, Name: "postgres", Ports: []int{5432}, Addresses: []string{"*:5432"}, StartedAt: started},
	}
}

func newTestModel(showSystem bool) Model {
	runner := &macostest.Runner{}
	return New(context.Background(), Config{Lister: ports.Lister{Runner: runner}, Stopper: ports.Stopper{Runner: runner}, ShowSystem: showSystem})
}

func update(t *testing.T, m Model, msgs ...tea.Msg) Model {
	t.Helper()
	for _, msg := range msgs {
		next, _ := m.Update(msg)
		m = next.(Model)
	}
	return m
}

func key(k string) tea.KeyPressMsg {
	switch k {
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	case "space":
		return tea.KeyPressMsg{Code: tea.KeySpace, Text: " "}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

func listed(processes []ports.Process) listedMsg {
	return listedMsg{processes: processes}
}

func TestSystemProcessesAreHiddenByDefault(t *testing.T) {
	m := update(t, newTestModel(false), listed(sampleProcesses()))

	if len(m.list.processes) != 2 || m.hidden != 1 {
		t.Errorf("listed %d, hidden %d; want 2 listed and 1 hidden", len(m.list.processes), m.hidden)
	}
	if !strings.Contains(m.selectingView(), "1 macOS system process hidden") {
		t.Error("the view does not mention the hidden system process")
	}

	m = update(t, newTestModel(true), listed(sampleProcesses()))
	if len(m.list.processes) != 3 {
		t.Errorf("with ShowSystem listed %d, want 3", len(m.list.processes))
	}
}

func TestEnterWithoutSelectionStopsTheCurrentProcess(t *testing.T) {
	m := update(t, newTestModel(false), listed(sampleProcesses()), key("down"), key("enter"))

	if m.state != stateConfirming || len(m.chosen) != 1 || m.chosen[0].Name != "postgres" {
		t.Fatalf("state %v, chosen %v; want to confirm postgres", m.state, m.chosen)
	}
	if !strings.Contains(m.confirmView(), "Stop 1 process?") {
		t.Error("confirmation does not ask to stop one process")
	}
}

func TestSelectionKeys(t *testing.T) {
	tests := []struct {
		name string
		keys []string
		want int
	}{
		{"nothing is preselected", nil, 0},
		{"toggle", []string{"space"}, 1},
		{"toggle twice", []string{"space", "space"}, 0},
		{"select all", []string{"a"}, 2},
		{"select none", []string{"a", "n"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := update(t, newTestModel(false), listed(sampleProcesses()))
			for _, k := range tt.keys {
				m = update(t, m, key(k))
			}
			if got := len(m.list.selectedProcesses()); got != tt.want {
				t.Errorf("selected %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRefreshKeepsSelection(t *testing.T) {
	m := update(t, newTestModel(false), listed(sampleProcesses()), key("down"), key("space"))

	m = update(t, m, key("r"))
	if m.state != stateLoading {
		t.Fatalf("state = %v, want loading after r", m.state)
	}
	m = update(t, m, listed(sampleProcesses()))

	chosen := m.list.selectedProcesses()
	if len(chosen) != 1 || chosen[0].PID != 77 {
		t.Errorf("selected %v after refresh, want postgres still selected", chosen)
	}
}

func TestDeclineReturnsToList(t *testing.T) {
	m := update(t, newTestModel(false), listed(sampleProcesses()), key("enter"), key("n"))

	if m.state != stateSelecting {
		t.Errorf("state = %v, want back to selecting", m.state)
	}
}

func TestSummaryAfterStopping(t *testing.T) {
	processes := sampleProcesses()
	m := update(t, newTestModel(false), listed(processes), key("a"), key("enter"), key("y"))
	m = update(t, m, stopDoneMsg{
		{Process: processes[0], Outcome: ports.Terminated},
		{Process: processes[2], Outcome: ports.Killed},
	})

	view := m.summaryView()
	if m.state != stateDone || !strings.Contains(view, "Stopped 2 processes") || !strings.Contains(view, "killed with SIGKILL") {
		t.Errorf("state %v, summary:\n%s", m.state, view)
	}
}

func TestNothingListening(t *testing.T) {
	m := update(t, newTestModel(false), listed(sampleProcesses()[1:2]))

	if m.state != stateDone || !strings.Contains(m.summaryView(), "Nothing is listening") {
		t.Errorf("state %v, summary %q", m.state, m.summaryView())
	}
}
