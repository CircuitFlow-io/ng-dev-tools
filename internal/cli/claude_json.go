package cli

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
)

type sessionsJSON struct {
	Sessions []sessionJSON     `json:"sessions"`
	Errors   map[string]string `json:"errors,omitempty"`
}

type sessionJSON struct {
	ID    string `json:"id"`
	Title string `json:"title,omitempty"`
	// Dir is the folder claude --resume must run in; DirExists is false once it moved or was deleted.
	Dir         string             `json:"dir"`
	DirExists   bool               `json:"dirExists"`
	File        string             `json:"file"`
	Status      sessionStatusJSON  `json:"status"`
	FirstPrompt string             `json:"firstPrompt"`
	LastPrompt  string             `json:"lastPrompt,omitempty"`
	Prompts     int                `json:"prompts"`
	Started     time.Time          `json:"started,omitzero"`
	LastActive  time.Time          `json:"lastActive,omitzero"`
	Branch      string             `json:"branch,omitempty"`
	Branches    []string           `json:"branches,omitempty"`
	Models      []sessionModelJSON `json:"models,omitempty"`
	PRs         []string           `json:"prs,omitempty"`
	SizeBytes   int64              `json:"sizeBytes"`
	Tokens      sessionTokensJSON  `json:"tokens"`
	Match       *sessionMatchJSON  `json:"match,omitempty"`
}

// sessionStatusJSON is what Claude is doing with a session open right now; closed sessions have
// only the activity.
type sessionStatusJSON struct {
	Activity   string    `json:"activity"`
	WaitingFor string    `json:"waitingFor,omitempty"`
	Since      time.Time `json:"since,omitzero"`
	PID        int       `json:"pid,omitempty"`
	Entrypoint string    `json:"entrypoint,omitempty"`
}

type sessionModelJSON struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Replies int    `json:"replies"`
}

// sessionTokensJSON has the context size at the latest reply, then totals over the session.
type sessionTokensJSON struct {
	Context    int64 `json:"context"`
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
}

// sessionMatchJSON is where a search matched in a prompt (yours) or a reply.
type sessionMatchJSON struct {
	Yours  bool   `json:"yours"`
	Before string `json:"before"`
	Match  string `json:"match"`
	After  string `json:"after"`
}

func toSessionsJSON(results []claudesessions.Result, live map[string]claudesessions.Live, errs map[string]error) sessionsJSON {
	views := make([]sessionJSON, 0, len(results))
	for _, r := range results {
		views = append(views, toSessionJSON(r, live[r.Session.ID]))
	}
	return sessionsJSON{Sessions: views, Errors: errorMessages(errs)}
}

func toSessionJSON(r claudesessions.Result, live claudesessions.Live) sessionJSON {
	s := r.Session
	return sessionJSON{
		ID:          s.ID,
		Title:       s.Title,
		Dir:         s.Dir,
		DirExists:   s.DirExists(),
		File:        s.File,
		Status:      toSessionStatusJSON(live),
		FirstPrompt: s.FirstPrompt,
		LastPrompt:  s.LastPrompt,
		Prompts:     s.Prompts,
		Started:     s.Started,
		LastActive:  s.LastActive,
		Branch:      s.Branch,
		Branches:    s.Branches,
		Models:      mapSlice(s.Models, toSessionModelJSON),
		PRs:         s.PRs,
		SizeBytes:   s.Size,
		Tokens:      toSessionTokensJSON(s.Usage),
		Match:       toSessionMatchJSON(r.Snippet),
	}
}

func toSessionStatusJSON(l claudesessions.Live) sessionStatusJSON {
	return sessionStatusJSON{
		Activity:   l.Activity.String(),
		WaitingFor: l.WaitingFor,
		Since:      l.Since,
		PID:        l.PID,
		Entrypoint: l.Entrypoint,
	}
}

func toSessionTokensJSON(u claudesessions.Usage) sessionTokensJSON {
	return sessionTokensJSON{Context: u.Context, Input: u.Input, Output: u.Output, CacheRead: u.CacheRead, CacheWrite: u.CacheWrite}
}

func toSessionModelJSON(m claudesessions.ModelUse) sessionModelJSON {
	return sessionModelJSON{ID: m.ID, Name: claudesessions.ModelName(m.ID), Replies: m.Replies}
}

func toSessionMatchJSON(s claudesessions.Snippet) *sessionMatchJSON {
	if s.IsZero() {
		return nil
	}
	return &sessionMatchJSON{Yours: s.Yours, Before: s.Before, Match: s.Match, After: s.After}
}
