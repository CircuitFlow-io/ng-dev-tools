// Package claudesessions reads Claude Code's saved sessions, across every project, and resumes one
// in the folder it belongs to.
package claudesessions

import (
	"cmp"
	"os"
	"slices"
	"time"
)

// Session is one saved Claude Code conversation.
type Session struct {
	ID   string
	File string
	// Dir is the folder the session belongs to, where claude --resume finds it.
	Dir string
	// Title is the name given with /rename, or else the one Claude generated.
	Title       string
	FirstPrompt string
	LastPrompt  string
	Prompts     int
	Started     time.Time
	LastActive  time.Time
	// Branch is the git branch at the latest message; Branches lists every one, in the order seen.
	Branch   string
	Branches []string
	// Models is how many replies each model wrote, the most used first.
	Models []ModelUse
	PRs    []string
	Size   int64
	// Turns is the text of the prompts and replies, for searching.
	Turns []Turn
}

// ModelUse is how many replies one model wrote in a session.
type ModelUse struct {
	ID      string
	Replies int
}

// Turn is the text of a prompt or a reply.
type Turn struct {
	Yours bool
	Text  string
}

// MainModel is the model that wrote most of the replies, or "" when there are none.
func (s Session) MainModel() string {
	if len(s.Models) == 0 {
		return ""
	}
	return s.Models[0].ID
}

// DirExists reports whether the session's folder is still there to resume in.
func (s Session) DirExists() bool {
	info, err := os.Stat(s.Dir)
	return err == nil && info.IsDir()
}

// Sort puts the most recently active sessions first.
func Sort(sessions []Session) {
	slices.SortStableFunc(sessions, func(a, b Session) int {
		return cmp.Or(b.LastActive.Compare(a.LastActive), cmp.Compare(a.ID, b.ID))
	})
}
