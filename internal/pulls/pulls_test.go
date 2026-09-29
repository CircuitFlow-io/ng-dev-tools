package pulls

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
)

const dashboardJSON = `{"data": {
  "viewer": {"login": "me", "pullRequests": {"nodes": [
    {"number": 2, "title": "Mine", "url": "https://github.com/acme/api/pull/2", "isDraft": false,
     "createdAt": "2026-09-01T10:00:00Z", "updatedAt": "2026-09-02T10:00:00Z", "headRefName": "feat", "baseRefName": "main",
     "additions": 10, "deletions": 3, "changedFiles": 2, "reviewDecision": "APPROVED", "mergeable": "MERGEABLE",
     "repository": {"nameWithOwner": "acme/api"}, "author": {"login": "me"}, "comments": {"totalCount": 1},
     "reviewRequests": {"nodes": [{"requestedReviewer": {"name": "backend"}}]},
     "latestReviews": {"nodes": [{"author": {"login": "sam"}, "state": "APPROVED"}]},
     "commits": {"nodes": [{"commit": {"statusCheckRollup": {"contexts": {"nodes": [
       {"__typename": "CheckRun", "name": "lint", "status": "COMPLETED", "conclusion": "SUCCESS", "detailsUrl": "https://github.com/acme/api/actions/runs/1/job/11", "checkSuite": {"workflowRun": {"workflow": {"name": "CI"}}}},
       {"__typename": "CheckRun", "name": "test", "status": "COMPLETED", "conclusion": "FAILURE", "detailsUrl": "https://github.com/acme/api/actions/runs/1/job/12", "checkSuite": {"workflowRun": {"workflow": {"name": "CI"}}}},
       {"__typename": "CheckRun", "name": "e2e", "status": "IN_PROGRESS", "conclusion": null, "detailsUrl": "", "checkSuite": null},
       {"__typename": "StatusContext", "context": "vercel", "state": "SUCCESS", "targetUrl": "https://vercel.com/x"}
     ]}}}}]}}
  ]}},
  "toReview": {"nodes": [
    {},
    {"number": 7, "title": "Older", "url": "https://github.com/acme/web/pull/7", "updatedAt": "2026-08-01T10:00:00Z",
     "repository": {"nameWithOwner": "acme/web"}, "author": null, "mergeable": "CONFLICTING", "reviewDecision": "REVIEW_REQUIRED",
     "comments": {"totalCount": 0}, "reviewRequests": {"nodes": []}, "latestReviews": {"nodes": []}, "commits": {"nodes": []}},
    {"number": 9, "title": "Newer", "url": "https://github.com/acme/web/pull/9", "updatedAt": "2026-09-10T10:00:00Z",
     "repository": {"nameWithOwner": "acme/web"}, "author": {"login": "kim"}, "mergeable": "MERGEABLE", "reviewDecision": "REVIEW_REQUIRED",
     "comments": {"totalCount": 0}, "reviewRequests": {"nodes": [{"requestedReviewer": {"login": "me"}}]}, "latestReviews": {"nodes": []}, "commits": {"nodes": []}}
  ]}
}}`

func TestParseDashboard(t *testing.T) {
	d, err := parseDashboard([]byte(dashboardJSON))
	if err != nil {
		t.Fatal(err)
	}
	if d.Viewer != "me" || len(d.Mine) != 1 || len(d.ToReview) != 2 {
		t.Fatalf("dashboard = %+v", d)
	}
	mine := d.Mine[0]
	if mine.Ref() != "acme/api#2" || mine.Requested[0] != "backend" || mine.Reviews[0] != (Review{Author: "sam", State: Approved}) {
		t.Errorf("mine = %+v", mine)
	}
	var names []string
	for _, c := range mine.Checks {
		names = append(names, c.Name)
	}
	if !slices.Equal(names, []string{"test", "e2e", "lint", "vercel"}) {
		t.Errorf("checks should be ordered failed, running, passed: %q", names)
	}
	if want := (CheckCounts{Failed: 1, Pending: 1, Passed: 2}); mine.CheckCounts() != want {
		t.Errorf("CheckCounts() = %+v, want %+v", mine.CheckCounts(), want)
	}
	if mine.Checks[0].Workflow != "CI" || mine.Checks[0].URL != "https://github.com/acme/api/actions/runs/1/job/12" {
		t.Errorf("failed check = %+v", mine.Checks[0])
	}
	if d.ToReview[0].Number != 9 || d.ToReview[1].Author != "ghost" {
		t.Errorf("reviews should be newest first, with a deleted author as ghost: %+v", d.ToReview)
	}
}

func TestParseDashboardReportsGraphQLErrors(t *testing.T) {
	_, err := parseDashboard([]byte(`{"errors": [{"message": "Something went wrong"}]}`))
	if err == nil || err.Error() != "Something went wrong" {
		t.Errorf("err = %v", err)
	}
}

func TestStatus(t *testing.T) {
	failing := []Check{{State: Failed}}
	running := []Check{{State: Pending}}
	tests := []struct {
		name string
		pr   PR
		want Status
	}{
		{"conflicts win", PR{Mergeable: conflicting, Checks: failing}, Conflicts},
		{"failed check", PR{Mergeable: mergeable, Checks: failing, ReviewDecision: ChangesRequested}, ChecksFailing},
		{"changes requested", PR{Mergeable: mergeable, ReviewDecision: ChangesRequested}, NeedsChanges},
		{"draft", PR{Mergeable: mergeable, Draft: true}, Draft},
		{"running", PR{Mergeable: mergeable, Checks: running}, ChecksRunning},
		{"approved and clean", PR{Mergeable: mergeable, ReviewDecision: Approved}, Ready},
		{"no review needed", PR{Mergeable: mergeable}, Ready},
		{"review required", PR{Mergeable: mergeable, ReviewDecision: ReviewRequired}, Waiting},
		{"conflicts unknown", PR{Mergeable: "UNKNOWN"}, Waiting},
	}
	for _, tt := range tests {
		if got := tt.pr.Status(); got != tt.want {
			t.Errorf("%s: Status() = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLoadThroughGH(t *testing.T) {
	query := "gh api graphql -f query=" + dashboardQuery
	runner := &macostest.Runner{Tools: map[string]bool{"gh": true}, Outputs: map[string]string{query: dashboardJSON}}
	d, err := Load(context.Background(), runner)
	if err != nil || d.Viewer != "me" {
		t.Fatalf("Load = %+v, %v", d, err)
	}

	if _, err := Load(context.Background(), &macostest.Runner{}); !errors.Is(err, ErrNoGH) {
		t.Errorf("without gh: %v", err)
	}
	loggedOut := &macostest.Runner{
		Tools:  map[string]bool{"gh": true},
		Errors: map[string]error{query: errors.New("gh api graphql: exit status 4: To get started with GitHub CLI, please run:  gh auth login")},
	}
	if _, err := Load(context.Background(), loggedOut); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("logged out: %v", err)
	}
}

func TestRepoFromURL(t *testing.T) {
	for url, want := range map[string]string{
		"git@github.com:acme/museum.git":       "acme/museum",
		"https://github.com/acme/home-app.git": "acme/home-app",
		"https://github.com/acme/api":          "acme/api",
		"ssh://git@github.com/acme/api.git":    "acme/api",
		"https://gitlab.com/acme/api.git":      "",
	} {
		got, ok := RepoFromURL(url)
		if got != want || ok != (want != "") {
			t.Errorf("RepoFromURL(%q) = %q, %v; want %q", url, got, ok, want)
		}
	}
}

func TestClonesFindsProjectsByRemote(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "museum", ".git", "config"), "[remote \"origin\"]\n\turl = git@github.com:acme/museum.git\n")
	writeFile(t, filepath.Join(root, "notes", "todo.txt"), "")
	clones := Clones(root)
	if dir, ok := LocalClone(clones, "acme/Museum"); !ok || dir != filepath.Join(root, "museum") {
		t.Errorf("LocalClone = %q, %v; clones %v", dir, ok, clones)
	}
	if len(clones) != 1 {
		t.Errorf("clones = %v, want only museum", clones)
	}
}

func TestFailedLog(t *testing.T) {
	check := Check{Name: "test", URL: "https://github.com/acme/api/actions/runs/1/job/12"}
	command := "gh run view --job 12 --repo acme/api --log-failed"
	runner := &macostest.Runner{Outputs: map[string]string{command: "" +
		"test\tRun tests\t2026-09-01T10:00:00.1234567Z \x1b[31mFAIL\x1b[0m pkg/api\n" +
		"test\tRun tests\t2026-09-01T10:00:01.0000000Z ##[error]Process completed with exit code 1.\n"}}
	lines, err := FailedLog(context.Background(), runner, check)
	if err != nil {
		t.Fatal(err)
	}
	want := []LogLine{{Step: "Run tests", Text: "FAIL pkg/api"}, {Step: "Run tests", Text: "##[error]Process completed with exit code 1."}}
	if !slices.Equal(lines, want) {
		t.Errorf("lines = %q", lines)
	}

	if _, err := FailedLog(context.Background(), runner, Check{URL: "https://vercel.com/x"}); !errors.Is(err, ErrNoLog) {
		t.Errorf("a check outside GitHub Actions: %v", err)
	}
	expired := &macostest.Runner{Errors: map[string]error{command: errors.New("failed to get run log: HTTP 410: Server Error")}}
	if _, err := FailedLog(context.Background(), expired, check); !errors.Is(err, ErrLogExpired) {
		t.Errorf("an expired log: %v", err)
	}
}

func TestCheckoutRefusesUncommittedChanges(t *testing.T) {
	dir := t.TempDir()
	runner := &macostest.Runner{Outputs: map[string]string{
		"git --no-optional-locks -C " + dir + " status --porcelain": " M main.go\n",
	}}
	if err := Checkout(context.Background(), runner, dir, PR{Repo: "acme/api", Number: 2}); !errors.Is(err, ErrUncommitted) {
		t.Errorf("Checkout = %v, want ErrUncommitted", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
