package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/standup"
)

func TestPrintStandupPassesTheStartAndPrintsTheReport(t *testing.T) {
	monday := time.Date(2026, time.September, 28, 0, 0, 0, 0, time.Local)
	var asked time.Time
	load := func(_ context.Context, since time.Time) (standup.Report, error) {
		asked = since
		return standup.Report{Since: since, Projects: []standup.Project{{
			Name:   "api",
			Groups: []standup.Group{{Branch: "spike", Commits: []standup.Commit{{Hash: "abcdef123", Subject: "Try things", At: monday.Add(time.Hour)}}}},
		}}}, nil
	}
	var out bytes.Buffer
	if err := printStandup(context.Background(), &out, load, monday); err != nil {
		t.Fatal(err)
	}
	if !asked.Equal(monday) {
		t.Errorf("loaded since %v, want %v", asked, monday)
	}
	for _, want := range []string{"Standup since Mon 28 Sep · 1 commit in 1 repo", "  spike  (not pushed)", "    abcdef1 Try things"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPrintStandupReturnsLoadErrors(t *testing.T) {
	load := func(context.Context, time.Time) (standup.Report, error) {
		return standup.Report{}, errors.New("no projects")
	}
	if err := printStandup(context.Background(), &bytes.Buffer{}, load, time.Time{}); err == nil {
		t.Error("want the load error")
	}
}
