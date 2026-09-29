package pulls

import (
	"cmp"
	"encoding/json"
	"errors"
	"slices"
	"time"
)

const checkRunKind = "CheckRun"

// dashboardQuery asks, in one request, for your open pull requests and the ones waiting for your
// review. The review list comes from search, the only way GitHub offers to find it.
const dashboardQuery = `query {
  viewer {
    login
    pullRequests(states: OPEN, first: 50, orderBy: {field: UPDATED_AT, direction: DESC}) { nodes { ...pr } }
  }
  toReview: search(query: "is:pr is:open review-requested:@me archived:false", type: ISSUE, first: 50) { nodes { ...pr } }
}

fragment pr on PullRequest {
  number title url body isDraft createdAt updatedAt headRefName baseRefName
  additions deletions changedFiles reviewDecision mergeable
  repository { nameWithOwner }
  author { login }
  comments { totalCount }
  reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } ... on Team { name } } } }
  latestReviews(first: 10) { nodes { author { login } state } }
  commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: 50) { nodes {
    __typename
    ... on CheckRun { name status conclusion detailsUrl checkSuite { workflowRun { workflow { name } } } }
    ... on StatusContext { context state targetUrl }
  } } } } } }
}`

// Dashboard is what involves the viewer: pull requests to review and their own.
type Dashboard struct {
	Viewer   string
	ToReview []PR
	Mine     []PR
}

type dashboardResponse struct {
	Data struct {
		Viewer struct {
			Login        string `json:"login"`
			PullRequests struct {
				Nodes []prNode `json:"nodes"`
			} `json:"pullRequests"`
		} `json:"viewer"`
		ToReview struct {
			Nodes []prNode `json:"nodes"`
		} `json:"toReview"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

type login struct {
	Login string `json:"login"`
}

type prNode struct {
	Number         int       `json:"number"`
	Title          string    `json:"title"`
	URL            string    `json:"url"`
	Body           string    `json:"body"`
	IsDraft        bool      `json:"isDraft"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	HeadRefName    string    `json:"headRefName"`
	BaseRefName    string    `json:"baseRefName"`
	Additions      int       `json:"additions"`
	Deletions      int       `json:"deletions"`
	ChangedFiles   int       `json:"changedFiles"`
	ReviewDecision string    `json:"reviewDecision"`
	Mergeable      string    `json:"mergeable"`
	Repository     struct {
		NameWithOwner string `json:"nameWithOwner"`
	} `json:"repository"`
	Author   *login `json:"author"`
	Comments struct {
		TotalCount int `json:"totalCount"`
	} `json:"comments"`
	ReviewRequests struct {
		Nodes []struct {
			RequestedReviewer struct {
				Login string `json:"login"`
				Name  string `json:"name"`
			} `json:"requestedReviewer"`
		} `json:"nodes"`
	} `json:"reviewRequests"`
	LatestReviews struct {
		Nodes []struct {
			Author *login `json:"author"`
			State  string `json:"state"`
		} `json:"nodes"`
	} `json:"latestReviews"`
	Commits struct {
		Nodes []struct {
			Commit struct {
				StatusCheckRollup *struct {
					Contexts struct {
						Nodes []contextNode `json:"nodes"`
					} `json:"contexts"`
				} `json:"statusCheckRollup"`
			} `json:"commit"`
		} `json:"nodes"`
	} `json:"commits"`
}

type contextNode struct {
	Typename   string `json:"__typename"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	DetailsURL string `json:"detailsUrl"`
	CheckSuite *struct {
		WorkflowRun *struct {
			Workflow struct {
				Name string `json:"name"`
			} `json:"workflow"`
		} `json:"workflowRun"`
	} `json:"checkSuite"`
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"targetUrl"`
}

func parseDashboard(data []byte) (Dashboard, error) {
	var resp dashboardResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return Dashboard{}, err
	}
	if len(resp.Errors) > 0 {
		return Dashboard{}, errors.New(resp.Errors[0].Message)
	}
	d := Dashboard{Viewer: resp.Data.Viewer.Login}
	for _, node := range resp.Data.Viewer.PullRequests.Nodes {
		d.Mine = append(d.Mine, node.pr())
	}
	for _, node := range resp.Data.ToReview.Nodes {
		if node.Number > 0 {
			d.ToReview = append(d.ToReview, node.pr())
		}
	}
	slices.SortStableFunc(d.ToReview, newestFirst)
	return d, nil
}

func newestFirst(a, b PR) int {
	return cmp.Or(b.UpdatedAt.Compare(a.UpdatedAt), cmp.Compare(a.Ref(), b.Ref()))
}

func (n prNode) pr() PR {
	p := PR{
		Repo:           n.Repository.NameWithOwner,
		Number:         n.Number,
		Title:          n.Title,
		URL:            n.URL,
		Body:           n.Body,
		Draft:          n.IsDraft,
		HeadRef:        n.HeadRefName,
		BaseRef:        n.BaseRefName,
		CreatedAt:      n.CreatedAt,
		UpdatedAt:      n.UpdatedAt,
		Additions:      n.Additions,
		Deletions:      n.Deletions,
		ChangedFiles:   n.ChangedFiles,
		Comments:       n.Comments.TotalCount,
		ReviewDecision: n.ReviewDecision,
		Mergeable:      n.Mergeable,
		Author:         loginOf(n.Author),
	}
	for _, r := range n.ReviewRequests.Nodes {
		p.Requested = append(p.Requested, cmp.Or(r.RequestedReviewer.Login, r.RequestedReviewer.Name))
	}
	for _, r := range n.LatestReviews.Nodes {
		p.Reviews = append(p.Reviews, Review{Author: loginOf(r.Author), State: r.State})
	}
	if len(n.Commits.Nodes) > 0 && n.Commits.Nodes[0].Commit.StatusCheckRollup != nil {
		for _, c := range n.Commits.Nodes[0].Commit.StatusCheckRollup.Contexts.Nodes {
			p.Checks = append(p.Checks, c.check())
		}
	}
	slices.SortStableFunc(p.Checks, func(a, b Check) int { return cmp.Compare(a.State, b.State) })
	return p
}

// loginOf names a GitHub account, or "ghost" for a deleted one, as GitHub itself does.
func loginOf(l *login) string {
	if l == nil {
		return "ghost"
	}
	return l.Login
}

func (c contextNode) check() Check {
	if c.Typename != checkRunKind {
		return Check{Name: c.Context, URL: c.TargetURL, State: statusContextState(c.State)}
	}
	check := Check{Name: c.Name, URL: c.DetailsURL, State: checkRunState(c.Status, c.Conclusion)}
	if c.CheckSuite != nil && c.CheckSuite.WorkflowRun != nil {
		check.Workflow = c.CheckSuite.WorkflowRun.Workflow.Name
	}
	return check
}
