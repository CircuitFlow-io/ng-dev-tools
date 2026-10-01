package cli

import (
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
)

type statusJSON struct {
	Repos []repoJSON `json:"repos"`
}

type repoJSON struct {
	Name            string       `json:"name"`
	Path            string       `json:"path"`
	Branch          string       `json:"branch,omitempty"`
	Detached        bool         `json:"detached,omitempty"`
	Unborn          bool         `json:"unborn,omitempty"`
	Upstream        string       `json:"upstream,omitempty"`
	UpstreamGone    bool         `json:"upstreamGone,omitempty"`
	Ahead           int          `json:"ahead"`
	Behind          int          `json:"behind"`
	HasRemote       bool         `json:"hasRemote"`
	Files           []fileJSON   `json:"files,omitempty"`
	LastCommit      commitJSON   `json:"lastCommit,omitzero"`
	Unpushed        int          `json:"unpushed"`
	UnpushedCommits []commitJSON `json:"unpushedCommits,omitempty"`
	Stashes         []stashJSON  `json:"stashes,omitempty"`
	Branches        []branchJSON `json:"branches,omitempty"`
	Operation       string       `json:"operation,omitempty"`
	FetchedAt       time.Time    `json:"fetchedAt,omitzero"`
	Error           string       `json:"error,omitempty"`
	FetchError      string       `json:"fetchError,omitempty"`
}

type fileJSON struct {
	Path string `json:"path"`
	From string `json:"from,omitempty"`
	// Code is the two-letter status as git status --short shows it.
	Code string `json:"code"`
}

type commitJSON struct {
	Hash    string    `json:"hash"`
	At      time.Time `json:"at,omitzero"`
	Subject string    `json:"subject"`
}

type stashJSON struct {
	Ref     string    `json:"ref"`
	At      time.Time `json:"at,omitzero"`
	Message string    `json:"message"`
}

type branchJSON struct {
	Name         string    `json:"name"`
	Upstream     string    `json:"upstream,omitempty"`
	UpstreamGone bool      `json:"upstreamGone,omitempty"`
	Ahead        int       `json:"ahead"`
	Behind       int       `json:"behind"`
	At           time.Time `json:"at,omitzero"`
}

func toStatusJSON(repos []gitstatus.Repo, fetchErrs map[string]error) statusJSON {
	views := make([]repoJSON, 0, len(repos))
	for _, r := range repos {
		view := toRepoJSON(r)
		view.FetchError = errText(fetchErrs[r.Path])
		views = append(views, view)
	}
	return statusJSON{Repos: views}
}

func toRepoJSON(r gitstatus.Repo) repoJSON {
	return repoJSON{
		Name:            r.Name,
		Path:            r.Path,
		Branch:          r.Branch,
		Detached:        r.Detached,
		Unborn:          r.Unborn,
		Upstream:        r.Upstream,
		UpstreamGone:    r.UpstreamGone,
		Ahead:           r.Ahead,
		Behind:          r.Behind,
		HasRemote:       r.HasRemote,
		Files:           mapSlice(r.Files, toFileJSON),
		LastCommit:      toCommitJSON(r.LastCommit),
		Unpushed:        r.Unpushed,
		UnpushedCommits: mapSlice(r.UnpushedCommits, toCommitJSON),
		Stashes:         mapSlice(r.Stashes, toStashJSON),
		Branches:        mapSlice(r.Branches, toBranchJSON),
		Operation:       strings.ToLower(r.Operation.Name()),
		FetchedAt:       r.FetchedAt,
		Error:           errText(r.Err),
	}
}

func toFileJSON(f gitstatus.File) fileJSON {
	return fileJSON{Path: f.Path, From: f.From, Code: f.Code()}
}

func toCommitJSON(c gitstatus.Commit) commitJSON {
	return commitJSON{Hash: c.Hash, At: c.At, Subject: c.Subject}
}

func toStashJSON(s gitstatus.Stash) stashJSON {
	return stashJSON{Ref: s.Ref, At: s.At, Message: s.Message}
}

func toBranchJSON(b gitstatus.Branch) branchJSON {
	return branchJSON{Name: b.Name, Upstream: b.Upstream, UpstreamGone: b.Gone, Ahead: b.Ahead, Behind: b.Behind, At: b.At}
}
