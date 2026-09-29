package cli

import (
	"bytes"
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos/macostest"
	"github.com/nasserghiasi/ng-dev-tools/internal/ports"
)

func TestParsePorts(t *testing.T) {
	got, err := parsePorts([]string{"3000", ":8080"})
	if err != nil || !slices.Equal(got, []int{3000, 8080}) {
		t.Errorf("parsePorts = %v, %v; want [3000 8080]", got, err)
	}
	for _, bad := range []string{"0", "65536", "http", "-1"} {
		if _, err := parsePorts([]string{bad}); err == nil {
			t.Errorf("parsePorts(%q) succeeded, want an error", bad)
		}
	}
}

func TestStopPortsReportsFreePortsAndRespectsDecline(t *testing.T) {
	runner := &macostest.Runner{Outputs: map[string]string{
		"lsof -nP -iTCP -sTCP:LISTEN -Fpcn": "p4242\ncnode\nn*:3000\n",
	}}
	var out bytes.Buffer
	var prompts []string
	decline := func(prompt string) (bool, error) {
		prompts = append(prompts, prompt)
		return false, nil
	}

	err := stopPorts(context.Background(), &out, ports.Lister{Runner: runner}, ports.Stopper{Runner: runner}, []int{3000, 9999}, decline)
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "Nothing is listening on :9999") || !strings.Contains(out.String(), "node (pid 4242) on :3000") {
		t.Errorf("output:\n%s", out.String())
	}
	if !slices.Equal(prompts, []string{"Stop 1 process? [y/N] "}) {
		t.Errorf("prompts = %q", prompts)
	}
	// The lister runs this once to describe the process; the stopper would run it again.
	if checks := countCalls(runner.Calls(), "ps -ww -o pid=,etime=,comm= -p 4242"); checks != 1 {
		t.Errorf("process looked up %d times, want once: nothing may be stopped after declining", checks)
	}
}

func TestStopPortsSkipsPromptWhenNothingListens(t *testing.T) {
	runner := &macostest.Runner{}
	var out bytes.Buffer
	asked := false
	confirm := func(string) (bool, error) { asked = true; return true, nil }

	if err := stopPorts(context.Background(), &out, ports.Lister{Runner: runner}, ports.Stopper{Runner: runner}, []int{3000}, confirm); err != nil {
		t.Fatal(err)
	}
	if asked {
		t.Error("asked for confirmation with nothing to stop")
	}
}

func TestConfirmerNeedsATerminalWithoutYes(t *testing.T) {
	var out bytes.Buffer
	if _, err := confirmer(strings.NewReader("y\n"), &out, false)("Stop? "); err == nil {
		t.Error("confirmed from a non-terminal reader, want errNeedsConfirmation")
	}
	if ok, err := confirmer(strings.NewReader(""), &out, true)("Stop? "); !ok || err != nil {
		t.Errorf("--yes = %v, %v; want approved", ok, err)
	}
}

func countCalls(calls []string, want string) int {
	var n int
	for _, call := range calls {
		if call == want {
			n++
		}
	}
	return n
}
