package cli

import (
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
)

type todosJSON struct {
	Items  []todoJSON        `json:"items"`
	Errors map[string]string `json:"errors,omitempty"`
}

type todoJSON struct {
	Project string `json:"project"`
	Dir     string `json:"dir"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	Marker  string `json:"marker"`
	Note    string `json:"note"`
	// Author, Email, At, Commit and Subject come from git blame and are left out for a line not
	// committed yet.
	Author      string    `json:"author,omitempty"`
	Email       string    `json:"email,omitempty"`
	At          time.Time `json:"at,omitzero"`
	Commit      string    `json:"commit,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	CommitURL   string    `json:"commitUrl,omitempty"`
	Uncommitted bool      `json:"uncommitted"`
	Mine        bool      `json:"mine"`
}

func toTodosJSON(items []todos.Item, errs map[string]error) todosJSON {
	views := make([]todoJSON, 0, len(items))
	for _, item := range items {
		views = append(views, toTodoJSON(item))
	}
	return todosJSON{Items: views, Errors: errorMessages(errs)}
}

func toTodoJSON(i todos.Item) todoJSON {
	return todoJSON{
		Project:     i.Project,
		Dir:         i.Dir,
		File:        i.File,
		Line:        i.Line,
		Marker:      strings.ToLower(string(i.Marker)),
		Note:        i.Note,
		Author:      i.Author,
		Email:       i.Email,
		At:          i.At,
		Commit:      i.Commit,
		Subject:     i.Subject,
		CommitURL:   i.CommitURL,
		Uncommitted: i.Uncommitted,
		Mine:        i.Mine,
	}
}
