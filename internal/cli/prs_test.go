package cli

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
)

func TestWritePRsListsReviewsThenYourOwn(t *testing.T) {
	now := time.Now()
	d := pulls.Dashboard{
		Viewer:   "me",
		ToReview: []pulls.PR{{Repo: "acme/web", Number: 9, Title: "Add search", Author: "kim", Mergeable: "CONFLICTING", UpdatedAt: now, URL: "https://github.com/acme/web/pull/9"}},
		Mine:     []pulls.PR{{Repo: "acme/api", Number: 2, Title: "Faster login", Author: "me", Mergeable: "MERGEABLE", UpdatedAt: now, URL: "https://github.com/acme/api/pull/2"}},
	}
	var out bytes.Buffer
	if err := writePRs(&out, d, now); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("want a header and two rows:\n%s", out.String())
	}
	if !strings.HasPrefix(lines[1], "review  acme/web#9") || !strings.Contains(lines[1], "conflicts") {
		t.Errorf("review row = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "mine    acme/api#2") || !strings.Contains(lines[2], "ready to merge") {
		t.Errorf("own row = %q", lines[2])
	}
}
