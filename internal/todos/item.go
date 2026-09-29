// Package todos finds the TODO, FIXME and HACK comments in the projects' git repositories and
// dates each one with git blame.
package todos

import (
	"cmp"
	"slices"
	"time"
)

// Item is one marker comment.
type Item struct {
	Project string
	// Dir is the repository; File is relative to it.
	Dir    string
	File   string
	Line   int
	Marker Marker
	Note   string
	// Author, Email, At, Commit and Subject come from git blame, and are empty for a line not
	// committed yet.
	Author      string
	Email       string
	At          time.Time
	Commit      string
	Subject     string
	Uncommitted bool
	// Mine is whether the line is yours, by your git email in that repository.
	Mine bool
	// CommitURL is the commit's page on GitHub, empty when it is not pushed or not on GitHub.
	CommitURL string
}

// Location is the file and line, as editors take them: apps/web/page.tsx:12.
func (i Item) Location() string {
	return i.File + ":" + itoa(i.Line)
}

// ID tells items apart across scans.
func (i Item) ID() string {
	return i.Dir + "\x00" + i.Location()
}

// Sort orders items oldest first, so the longest forgotten lead; lines not committed yet go last.
func Sort(items []Item) {
	slices.SortStableFunc(items, func(a, b Item) int {
		if a.Uncommitted != b.Uncommitted {
			if a.Uncommitted {
				return 1
			}
			return -1
		}
		return cmp.Or(a.At.Compare(b.At), cmp.Compare(a.Project, b.Project), cmp.Compare(a.File, b.File), cmp.Compare(a.Line, b.Line))
	})
}
