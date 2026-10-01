package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/doctor"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
)

// encode writes v as the commands do and decodes it back into a generic tree.
func encode(t *testing.T, v any) map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := writeJSON(&out, v); err != nil {
		t.Fatal(err)
	}
	var tree map[string]any
	if err := json.Unmarshal(out.Bytes(), &tree); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	return tree
}

func TestEveryCommandAcceptsJSON(t *testing.T) {
	var visit func(cmd *cobra.Command)
	visit = func(cmd *cobra.Command) {
		if cmd.Flag(jsonFlag) == nil {
			t.Errorf("%s has no --%s flag", cmd.CommandPath(), jsonFlag)
		}
		for _, sub := range cmd.Commands() {
			visit(sub)
		}
	}
	visit(NewRootCmd("test"))
}

func TestStatusJSONNamesTheOperationAndOmitsWhatIsUnset(t *testing.T) {
	repos := []gitstatus.Repo{{Name: "web", Path: "/p/web", Operation: gitstatus.Rebase, Files: []gitstatus.File{{Path: "a.go", Staged: 'M', Unstaged: '.'}}}}
	tree := encode(t, toStatusJSON(repos, map[string]error{"/p/web": errors.New("auth failed")}))

	repo := tree["repos"].([]any)[0].(map[string]any)
	if repo["operation"] != "rebase" || repo["fetchError"] != "auth failed" {
		t.Errorf("repo = %v", repo)
	}
	if code := repo["files"].([]any)[0].(map[string]any)["code"]; code != "M " {
		t.Errorf("file code = %q, want %q", code, "M ")
	}
	for _, key := range []string{"error", "lastCommit", "fetchedAt", "stashes"} {
		if _, ok := repo[key]; ok {
			t.Errorf("%s is set on a repository without one: %v", key, repo[key])
		}
	}
}

func TestPRsJSONKeepsEmptyGroupsAsArrays(t *testing.T) {
	d := pulls.Dashboard{Viewer: "me", Mine: []pulls.PR{{
		Repo: "acme/api", Number: 2, Author: "me", Mergeable: "MERGEABLE",
		Checks: []pulls.Check{{Name: "test", State: pulls.Failed}},
	}}}
	tree := encode(t, toPRsJSON(d))

	if review, ok := tree["toReview"].([]any); !ok || len(review) != 0 {
		t.Errorf("toReview = %#v, want []", tree["toReview"])
	}
	pr := tree["mine"].([]any)[0].(map[string]any)
	if pr["state"] != "checks failing" {
		t.Errorf("state = %v", pr["state"])
	}
	if check := pr["checks"].([]any)[0].(map[string]any); check["state"] != "failed" {
		t.Errorf("check = %v", check)
	}
}

func TestDoctorJSONCountsEveryCheckButListsOnlyProblems(t *testing.T) {
	outcomes := []doctor.Outcome{
		{Check: doctor.Check{Name: "Go", Group: doctor.GroupGo}, Result: doctor.Result{Status: doctor.StatusPass}},
		{Check: doctor.Check{Name: "dlv", Group: doctor.GroupGo}, Result: doctor.Result{Status: doctor.StatusFail, Fix: "go install dlv"}, Duration: 2 * time.Second},
	}
	tree := encode(t, toDoctorJSON(outcomes, true))

	if counts := tree["counts"].(map[string]any); counts["pass"] != 1.0 || counts["fail"] != 1.0 {
		t.Errorf("counts = %v", counts)
	}
	checks := tree["checks"].([]any)
	if len(checks) != 1 {
		t.Fatalf("checks = %v, want only the failure", checks)
	}
	check := checks[0].(map[string]any)
	if check["group"] != "go" || check["status"] != "fail" || check["durationMs"] != 2000.0 {
		t.Errorf("check = %v", check)
	}
}

func TestStopPortsJSONNeedsYesAndPrintsNothingWithout(t *testing.T) {
	runner := &macostest.Runner{}
	var out bytes.Buffer
	err := stopPortsJSON(context.Background(), &out, ports.Lister{Runner: runner}, ports.Stopper{Runner: runner}, []int{3000}, false)
	if !errors.Is(err, errJSONNeedsConfirmation) {
		t.Errorf("err = %v, want errJSONNeedsConfirmation", err)
	}
	if out.Len() > 0 || len(runner.Calls()) > 0 {
		t.Errorf("did something without --yes: output %q, calls %v", out.String(), runner.Calls())
	}
}

func TestStopPortsJSONReportsFreePorts(t *testing.T) {
	runner := &macostest.Runner{}
	var out bytes.Buffer
	if err := stopPortsJSON(context.Background(), &out, ports.Lister{Runner: runner}, ports.Stopper{Runner: runner}, []int{9999}, true); err != nil {
		t.Fatal(err)
	}
	var got stopJSON
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Free) != 1 || got.Free[0] != 9999 || got.Results == nil || len(got.Results) != 0 {
		t.Errorf("got %+v, want :9999 free and no results", got)
	}
}

func TestRunJSONRefusesLast(t *testing.T) {
	err := runRun(context.Background(), io.Discard, io.Discard, outputJSON, nil, runFlags{last: true})
	if !errors.Is(err, errJSONRunsNone) {
		t.Errorf("err = %v, want errJSONRunsNone", err)
	}
}

func TestTodosJSONLowercasesMarkersAndKeepsErrors(t *testing.T) {
	items := []todos.Item{{Project: "web", File: "a.go", Line: 3, Marker: todos.Fixme, Uncommitted: true}}
	tree := encode(t, toTodosJSON(items, map[string]error{"api": errors.New("git failed")}))

	item := tree["items"].([]any)[0].(map[string]any)
	if item["marker"] != "fixme" || item["uncommitted"] != true {
		t.Errorf("item = %v", item)
	}
	if _, ok := item["at"]; ok {
		t.Error("an uncommitted line has a blame date")
	}
	if errs := tree["errors"].(map[string]any); errs["api"] != "git failed" {
		t.Errorf("errors = %v", errs)
	}
}

func TestSessionsJSONLeavesOutTheConversation(t *testing.T) {
	session := claudesessions.Session{ID: "abc", FirstPrompt: "hi", Turns: []claudesessions.Turn{{Text: "a secret reply"}}}
	results := []claudesessions.Result{{Session: session, Snippet: claudesessions.Snippet{Match: "hi", Yours: true}}}
	live := map[string]claudesessions.Live{"abc": {Activity: claudesessions.Waiting, WaitingFor: "permission"}}

	var out bytes.Buffer
	if err := writeJSON(&out, toSessionsJSON(results, live, nil)); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "a secret reply") {
		t.Errorf("the transcript leaked into the output:\n%s", out.String())
	}
	tree := encode(t, toSessionsJSON(results, live, nil))
	s := tree["sessions"].([]any)[0].(map[string]any)
	if status := s["status"].(map[string]any); status["activity"] != "waiting" || status["waitingFor"] != "permission" {
		t.Errorf("status = %v", status)
	}
	if match := s["match"].(map[string]any); match["match"] != "hi" {
		t.Errorf("match = %v", match)
	}
}

func TestEnvJSONNamesTheAttention(t *testing.T) {
	sets := []envfiles.Set{{Name: "web", Example: ".env.example", Locals: []string{".env"}, Missing: []string{"API_KEY"}}}
	set := encode(t, toEnvJSON(sets))["sets"].([]any)[0].(map[string]any)
	if set["attention"] != "missing" || set["missing"].([]any)[0] != "API_KEY" {
		t.Errorf("set = %v", set)
	}
}
