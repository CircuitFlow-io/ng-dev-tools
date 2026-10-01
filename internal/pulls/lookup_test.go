package pulls

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos/macostest"
)

func lookupCommand(urls ...string) string {
	line := "gh api graphql -f query=" + lookupQuery(len(urls))
	for i, url := range urls {
		line += fmt.Sprintf(" -f u%d=%s", i, url)
	}
	return line
}

func TestLookupFindsPullRequestsByURL(t *testing.T) {
	const (
		merged    = "https://github.com/acme/api/pull/9"
		draft     = "https://github.com/acme/web/pull/12"
		gone      = "https://github.com/acme/api/pull/404"
		issue     = "https://github.com/acme/api/pull/7"
		notGitHub = "https://gitlab.com/acme/api/-/merge_requests/3"
	)
	answer := `{"data":{
		"p0":{"number":9,"title":"Add sessions list","state":"MERGED","isDraft":false,"repository":{"nameWithOwner":"acme/api"}},
		"p1":{"number":12,"title":"Try a thing","state":"OPEN","isDraft":true,"repository":{"nameWithOwner":"acme/web"}},
		"p2":null,
		"p3":{}}}`
	runner := &macostest.Runner{
		Tools:   map[string]bool{"gh": true},
		Outputs: map[string]string{lookupCommand(merged, draft, gone, issue): answer},
	}

	found, err := Lookup(context.Background(), runner, []string{merged, draft, merged, notGitHub, gone, issue})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Summary{
		merged: {Repo: "acme/api", Number: 9, Title: "Add sessions list", State: StateMerged},
		draft:  {Repo: "acme/web", Number: 12, Title: "Try a thing", State: StateOpen, Draft: true},
	}
	if len(found) != len(want) || found[merged] != want[merged] || found[draft] != want[draft] {
		t.Errorf("found = %+v, want %+v", found, want)
	}
}

func TestLookupAsksInBatches(t *testing.T) {
	urls := make([]string, lookupBatch+1)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://github.com/acme/api/pull/%d", i+1)
	}
	runner := &macostest.Runner{Tools: map[string]bool{"gh": true}, Outputs: map[string]string{
		lookupCommand(urls[:lookupBatch]...): `{"data":{}}`,
		lookupCommand(urls[lookupBatch:]...): `{"data":{"p0":{"number":51,"title":"Last","state":"CLOSED","repository":{"nameWithOwner":"acme/api"}}}}`,
	}}

	found, err := Lookup(context.Background(), runner, urls)
	if err != nil || len(runner.Calls()) != 2 || found[urls[lookupBatch]].State != StateClosed {
		t.Errorf("found %v, %v after %d requests", found, err, len(runner.Calls()))
	}
}

func TestLookupFailures(t *testing.T) {
	url := "https://github.com/acme/api/pull/9"
	if _, err := Lookup(context.Background(), &macostest.Runner{}, []string{url}); !errors.Is(err, ErrNoGH) {
		t.Errorf("without gh: %v", err)
	}
	loggedOut := &macostest.Runner{
		Tools:  map[string]bool{"gh": true},
		Errors: map[string]error{lookupCommand(url): errors.New("exit status 4: please run:  gh auth login")},
	}
	if _, err := Lookup(context.Background(), loggedOut, []string{url}); !errors.Is(err, ErrNotLoggedIn) {
		t.Errorf("logged out: %v", err)
	}
	refused := &macostest.Runner{
		Tools:   map[string]bool{"gh": true},
		Outputs: map[string]string{lookupCommand(url): `{"errors":[{"message":"rate limited"}]}`},
	}
	if _, err := Lookup(context.Background(), refused, []string{url}); err == nil || !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("GraphQL error: %v", err)
	}
}

func TestParseURL(t *testing.T) {
	repo, number, ok := ParseURL("https://github.com/CircuitFlow-io/ng-dev-tools/pull/13")
	if repo != "CircuitFlow-io/ng-dev-tools" || number != 13 || !ok {
		t.Errorf("ParseURL = %q, %d, %v", repo, number, ok)
	}
	for _, url := range []string{"https://github.com/acme/api/issues/3", "https://github.com/acme/api/pull/x", "http://example.com/a/b/pull/1"} {
		if _, _, ok := ParseURL(url); ok {
			t.Errorf("ParseURL(%q) accepted it", url)
		}
	}
}
