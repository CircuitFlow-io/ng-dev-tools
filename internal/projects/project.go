// Package projects finds the projects in a folder and orders them by recent activity.
package projects

import (
	"strings"
	"time"
	"unicode/utf8"
)

// Project is a folder holding one codebase.
type Project struct {
	// Name is the path relative to the projects folder, e.g. "blog/blog-app".
	Name    string
	Path    string
	Git     bool
	Branch  string
	Opened  time.Time
	Changed time.Time
}

// LastActivity is when the project was last opened with ngt or had a file change.
func (p Project) LastActivity() time.Time {
	if p.Opened.After(p.Changed) {
		return p.Opened
	}
	return p.Changed
}

// ActivityVerb says which kind of activity LastActivity is.
func (p Project) ActivityVerb() string {
	if p.Opened.After(p.Changed) {
		return "opened"
	}
	return "changed"
}

// Matches reports whether query's characters appear in name in order, ignoring case,
// so "bba" matches "blog/blog-app".
func Matches(name, query string) bool {
	name, query = strings.ToLower(name), strings.ToLower(query)
	for query != "" {
		r, size := utf8.DecodeRuneInString(query)
		i := strings.IndexRune(name, r)
		if i < 0 {
			return false
		}
		name = name[i+utf8.RuneLen(r):]
		query = query[size:]
	}
	return true
}

// Filter keeps the projects whose names match query, in order.
func Filter(projects []Project, query string) []Project {
	var kept []Project
	for _, p := range projects {
		if Matches(p.Name, query) {
			kept = append(kept, p)
		}
	}
	return kept
}
