package pulls

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

// Pull request states as GitHub names them.
const (
	StateOpen   = "OPEN"
	StateMerged = "MERGED"
	StateClosed = "CLOSED"
)

// lookupBatch is how many pull requests one request looks up, well inside GitHub's query limits.
const lookupBatch = 50

var prURL = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/pull/(\d+)/?$`)

// Summary is where a pull request stands.
type Summary struct {
	// Repo is "owner/name".
	Repo   string
	Number int
	Title  string
	// State is StateOpen, StateMerged or StateClosed.
	State string
	Draft bool
}

// ParseURL reads a github.com pull request URL into its repository and number.
func ParseURL(url string) (repo string, number int, ok bool) {
	m := prURL.FindStringSubmatch(url)
	if m == nil {
		return "", 0, false
	}
	number, err := strconv.Atoi(m[2])
	return m[1], number, err == nil
}

// Lookup finds the pull requests at urls, by URL, a batch per request through gh. URLs that are
// not github.com pull requests are left out, and so are pull requests that are gone or that you
// cannot see.
func Lookup(ctx context.Context, runner macos.Runner, urls []string) (map[string]Summary, error) {
	if !runner.Available("gh") {
		return nil, ErrNoGH
	}
	found := map[string]Summary{}
	for batch := range slices.Chunk(pullRequestURLs(urls), lookupBatch) {
		args := []string{"api", "graphql", "-f", "query=" + lookupQuery(len(batch))}
		for i, url := range batch {
			args = append(args, "-f", fmt.Sprintf("u%d=%s", i, url))
		}
		out, err := runner.Run(ctx, "gh", args...)
		if err != nil {
			return found, ghError(err)
		}
		if err := parseLookup(out, batch, found); err != nil {
			return found, err
		}
	}
	return found, nil
}

// pullRequestURLs keeps each github.com pull request URL once, in order.
func pullRequestURLs(urls []string) []string {
	var kept []string
	for _, url := range urls {
		if _, _, ok := ParseURL(url); ok && !slices.Contains(kept, url) {
			kept = append(kept, url)
		}
	}
	return kept
}

// lookupQuery asks for n pull requests by URL, passed as the variables $u0 to $u(n-1) so the URLs
// never become part of the query's text.
func lookupQuery(n int) string {
	var params, fields []string
	for i := range n {
		params = append(params, fmt.Sprintf("$u%d: URI!", i))
		fields = append(fields, fmt.Sprintf("  p%d: resource(url: $u%d) { ...summary }", i, i))
	}
	return "query(" + strings.Join(params, ", ") + ") {\n" + strings.Join(fields, "\n") + "\n}\n\n" +
		"fragment summary on PullRequest { number title state isDraft repository { nameWithOwner } }"
}

type summaryNode struct {
	Number     int    `json:"number"`
	Title      string `json:"title"`
	State      string `json:"state"`
	IsDraft    bool   `json:"isDraft"`
	Repository struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
}

// parseLookup adds the pull requests found for batch to found. GitHub answers null for a URL it
// does not know and {} for one that is not a pull request.
func parseLookup(out []byte, batch []string, found map[string]Summary) error {
	var resp struct {
		Data   map[string]*summaryNode `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		return fmt.Errorf("reading GitHub's answer: %w", err)
	}
	if resp.Data == nil && len(resp.Errors) > 0 {
		return fmt.Errorf("GitHub: %s", resp.Errors[0].Message)
	}
	for i, url := range batch {
		node := resp.Data["p"+strconv.Itoa(i)]
		if node == nil || node.Number == 0 {
			continue
		}
		found[url] = Summary{
			Repo:   node.Repository.NameWithOwner,
			Number: node.Number,
			Title:  node.Title,
			State:  node.State,
			Draft:  node.IsDraft,
		}
	}
	return nil
}
