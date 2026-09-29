package standup

import (
	"slices"
	"testing"
	"time"
)

// tuesday is Tuesday 29 September 2026, 10:00.
var tuesday = time.Date(2026, time.September, 29, 10, 0, 0, 0, time.UTC)

// september is the start of that day in September 2026.
func september(d int) time.Time {
	return time.Date(2026, time.September, d, 0, 0, 0, 0, time.UTC)
}

func TestParseSince(t *testing.T) {
	tests := []struct {
		value string
		want  time.Time
	}{
		{"today", september(29)},
		{"yesterday", september(28)},
		{"monday", september(28)},
		{"Tue", september(29)},
		{"wednesday", september(23)},
		{"2026-09-01", september(1)},
		{"3d", september(26)},
		{"2w", september(15)},
	}
	for _, tt := range tests {
		got, err := ParseSince(tt.value, tuesday)
		if err != nil || !got.Equal(tt.want) {
			t.Errorf("ParseSince(%q) = %v, %v; want %v", tt.value, got, err, tt.want)
		}
	}
	for _, bad := range []string{"", "soon", "d", "-3d", "2026-10-30", "mo"} {
		if _, err := ParseSince(bad, tuesday); err == nil {
			t.Errorf("ParseSince(%q) did not fail", bad)
		}
	}
}

func TestLastWorkedDay(t *testing.T) {
	monday := time.Date(2026, time.September, 28, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name    string
		now     time.Time
		commits []time.Time
		want    time.Time
	}{
		{"the newest day before today", tuesday, []time.Time{september(24).Add(time.Hour), tuesday.Add(-time.Hour)}, september(24)},
		{"a weekend you worked", monday, []time.Time{september(27).Add(3 * time.Hour)}, september(27)},
		{"nothing falls back to Friday on a Monday", monday, nil, september(25)},
		{"nothing falls back to yesterday midweek", tuesday, nil, september(28)},
	}
	for _, tt := range tests {
		if got := lastWorkedDay(tt.commits, tt.now); !got.Equal(tt.want) {
			t.Errorf("%s: lastWorkedDay = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestBranchName(t *testing.T) {
	tests := []struct {
		ref, want string
		ok        bool
	}{
		{"refs/heads/feat/x", "feat/x", true},
		{"refs/remotes/origin/feat/x", "feat/x", true},
		{"refs/remotes/origin/HEAD", "", false},
		{"refs/tags/v1", "", false},
	}
	for _, tt := range tests {
		if got, ok := branchName(tt.ref); got != tt.want || ok != tt.ok {
			t.Errorf("branchName(%q) = %q, %v; want %q, %v", tt.ref, got, ok, tt.want, tt.ok)
		}
	}
}

const activityJSON = `{"data": {
  "viewer": {"login": "me"},
  "authored": {"nodes": [
    {"number": 5, "title": "Add todo", "url": "https://github.com/acme/api/pull/5", "state": "MERGED",
     "createdAt": "2026-09-28T10:00:00Z", "mergedAt": "2026-09-29T09:00:00Z", "closedAt": "2026-09-29T09:00:00Z",
     "headRefName": "feat/todo", "repository": {"nameWithOwner": "acme/api"},
     "commits": {"totalCount": 1, "nodes": [{"commit": {"oid": "aaa"}}]}},
    {}
  ]},
  "reviewed": {"nodes": [
    {"number": 9, "title": "Fix login", "url": "u9", "state": "OPEN", "repository": {"nameWithOwner": "acme/web"},
     "author": {"login": "kim"},
     "reviews": {"nodes": [
       {"author": {"login": "me"}, "state": "APPROVED", "submittedAt": "2026-09-28T11:00:00Z"},
       {"author": {"login": "me"}, "state": "COMMENTED", "submittedAt": "2026-09-28T12:00:00Z"},
       {"author": {"login": "sam"}, "state": "CHANGES_REQUESTED", "submittedAt": "2026-09-28T13:00:00Z"}
     ]},
     "comments": {"nodes": []}}
  ]},
  "commented": {"nodes": [
    {"number": 9, "title": "Fix login", "url": "u9", "state": "OPEN", "repository": {"nameWithOwner": "acme/web"},
     "author": {"login": "kim"}, "reviews": {"nodes": []},
     "comments": {"nodes": [{"author": {"login": "me"}, "createdAt": "2026-09-28T14:00:00Z"}]}},
    {"number": 3, "title": "Old talk", "url": "u3", "state": "OPEN", "repository": {"nameWithOwner": "acme/web"},
     "author": null, "reviews": {"nodes": []},
     "comments": {"nodes": [{"author": {"login": "me"}, "createdAt": "2026-09-01T14:00:00Z"}]}},
    {"number": 4, "title": "Just a comment", "url": "u4", "state": "MERGED", "repository": {"nameWithOwner": "acme/web"},
     "author": null, "reviews": {"nodes": []},
     "comments": {"nodes": [{"author": {"login": "me"}, "createdAt": "2026-09-28T08:00:00Z"}]}}
  ]}
}}`

func TestParseActivity(t *testing.T) {
	g, err := parseActivity([]byte(activityJSON), september(28))
	if err != nil {
		t.Fatal(err)
	}
	if g.viewer != "me" || len(g.authored) != 1 {
		t.Fatalf("got %+v", g)
	}
	pr := g.authored[0]
	if pr.Ref() != "acme/api#5" || pr.Branch != "feat/todo" || !pr.commits["aaa"] || pr.MergedAt.IsZero() {
		t.Errorf("authored = %+v", pr)
	}
	var got []string
	for _, r := range g.reviewed {
		got = append(got, r.Ref()+" "+r.Verdict+" by "+r.Author)
	}
	want := []string{"acme/web#9 APPROVED by kim", "acme/web#4 COMMENTED by ghost"}
	if !slices.Equal(got, want) {
		t.Errorf("reviewed = %q, want %q: an approval outweighs a later comment, older comments are left out", got, want)
	}
}

func TestParseActivityReportsGraphQLErrors(t *testing.T) {
	if _, err := parseActivity([]byte(`{"errors": [{"message": "rate limited"}]}`), tuesday); err == nil || err.Error() != "rate limited" {
		t.Errorf("err = %v", err)
	}
}

func TestBuildGroupsCommitsUnderTheirPullRequestOrBranch(t *testing.T) {
	since := september(28)
	at := func(hour int) time.Time { return since.Add(time.Duration(hour) * time.Hour) }
	prs := []PR{
		{Repo: "acme/api", Number: 5, Branch: "feat/b", State: Merged, CreatedAt: at(1), MergedAt: at(9), commits: map[string]bool{"a": true, "b": true}, commitCount: 2},
		{Repo: "acme/api", Number: 4, Branch: "feat/a", State: Merged, CreatedAt: at(1), MergedAt: at(8), commits: map[string]bool{"a": true}, commitCount: 1},
		{Repo: "acme/api", Number: 6, Branch: "feat/wip", State: Open, CreatedAt: since.AddDate(0, 0, -5)},
		{Repo: "acme/api", Number: 7, Branch: "feat/idle", State: Open, CreatedAt: since.AddDate(0, 0, -5)},
		{Repo: "acme/api", Number: 8, Branch: "feat/done", State: Merged, CreatedAt: since.AddDate(0, 0, -5), MergedAt: at(2)},
		{Repo: "acme/docs", Number: 1, Branch: "typo", State: Open, CreatedAt: at(3)},
	}
	work := []localWork{{
		repo: repo{name: "api", dir: "/p/api", github: "acme/api", defaultBranch: "main"},
		commits: []Commit{
			{Hash: "w", At: at(7)}, {Hash: "s", At: at(6)}, {Hash: "b", At: at(5)}, {Hash: "a", At: at(4)}, {Hash: "m", At: at(3)},
		},
		branchOf: map[string]string{"w": "feat/wip", "s": "spike"},
	}, {
		repo: repo{name: "quiet", dir: "/p/quiet", defaultBranch: "main"},
	}}

	projects := build(since, work, prs)
	var got []string
	for _, p := range projects {
		got = append(got, "project "+p.Name)
		for _, g := range p.Groups {
			label := "branch " + g.Branch
			if g.PR != nil {
				label = g.PR.Ref()
			}
			for _, c := range g.Commits {
				label += " " + c.Hash
			}
			got = append(got, label)
		}
	}
	want := []string{
		"project api",
		"acme/api#5 b",
		"acme/api#4 a",
		"acme/api#6 w",
		"branch spike s",
		"branch  m",
		"acme/api#8",
		"project acme/docs",
		"acme/docs#1",
	}
	if !slices.Equal(got, want) {
		t.Errorf("groups:\n got %q\nwant %q", got, want)
	}
	if projects[1].Dir != "" || projects[1].URL != "https://github.com/acme/docs" {
		t.Errorf("remote-only project = %+v", projects[1])
	}
}

func TestBuildShowsCommitsAndPullRequestsOnceAcrossClones(t *testing.T) {
	since := september(28)
	prs := []PR{{Repo: "acme/api", Number: 5, State: Merged, CreatedAt: since.Add(time.Hour), MergedAt: since.Add(2 * time.Hour)}}
	clone := func(name string) localWork {
		return localWork{
			repo:    repo{name: name, dir: "/p/" + name, github: "acme/api", defaultBranch: "main"},
			commits: []Commit{{Hash: "a", At: since.Add(time.Hour)}},
		}
	}
	projects := build(since, []localWork{clone("api"), clone("api-copy")}, prs)
	if len(projects) != 1 || projects[0].Name != "api" || len(projects[0].Groups) != 2 {
		t.Errorf("projects = %+v, want the commit and the pull request once, in the first clone", projects)
	}
}

func TestReportCounts(t *testing.T) {
	since := september(28)
	opened := PR{CreatedAt: since.Add(time.Hour)}
	merged := PR{CreatedAt: since.AddDate(0, 0, -3), MergedAt: since.Add(time.Hour)}
	r := Report{Since: since, Projects: []Project{{Groups: []Group{
		{PR: &opened, Commits: []Commit{{Hash: "a"}, {Hash: "b"}}},
		{PR: &merged},
		{Branch: "x", Commits: []Commit{{Hash: "c"}}},
	}}}}
	if n := r.Commits(); n != 3 {
		t.Errorf("Commits() = %d", n)
	}
	if o, m := r.PRs(); o != 1 || m != 1 {
		t.Errorf("PRs() = %d opened, %d merged", o, m)
	}
}
