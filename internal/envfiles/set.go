// Package envfiles compares a project's env files with the examples that document them, by key
// name only: values are never read into memory beyond telling whether they are empty.
package envfiles

import (
	"cmp"
	"path/filepath"
	"slices"
	"time"
)

// Set is an example file and the local files it describes, in one folder. A folder whose local
// files have no example forms a set without one.
type Set struct {
	// Name is the folder relative to the projects root, such as museum/apps/web.
	Name string
	Dir  string
	// Example is the example's file name, empty when the local files have none.
	Example     string
	ExampleKeys int
	// Locals are the files holding your values; Defaults are shared ones committed on purpose.
	Locals   []string
	Defaults []string
	// Unread are local files that are not regular files, such as the named pipes 1Password
	// Environments serves secrets through. They are never opened, since reading one hands over
	// its secrets, so what keys they hold is unknown.
	Unread []string
	// Missing are the example's keys that no local or defaults file sets, in the example's order.
	Missing []string
	// Extra are keys of the local files that the example does not list.
	Extra []KeyInFile
	Empty []KeyInFile
	// InCode are keys the code reads that neither the example nor any readable local file has.
	InCode []CodeRef
	// Tracked are local files git tracks; Committed are ones it no longer tracks but that are
	// still in its history.
	Tracked   []string
	Committed []Commit
	Err       error
}

// KeyInFile is a key and the local file it is in.
type KeyInFile struct {
	Key  string
	File string
}

// CodeRef is where the code first reads a key, relative to the set's folder.
type CodeRef struct {
	Key  string
	File string
	Line int
}

// Commit is the latest commit that added a local env file.
type Commit struct {
	File string
	Hash string
	At   time.Time
}

// Level is how urgently a set needs attention, most urgent first.
type Level int

const (
	// Exposed sets have a local file that is or was committed, so its secrets may have leaked.
	Exposed Level = iota
	Missing
	Empty
	// Undocumented sets use keys the example does not list.
	Undocumented
	NotSetUp
	NoExample
	// Unchecked sets have a local file that cannot be read without handing over its secrets.
	Unchecked
	Complete
)

// NeedsAttention reports whether the level is something to act on.
func (l Level) NeedsAttention() bool {
	return l < NotSetUp
}

// Attention is the most urgent thing about the set.
func (s Set) Attention() Level {
	switch {
	case s.Err != nil, len(s.Tracked) > 0, len(s.Committed) > 0:
		return Exposed
	case len(s.Missing) > 0:
		return Missing
	case len(s.Empty) > 0:
		return Empty
	case len(s.Extra) > 0, len(s.InCode) > 0:
		return Undocumented
	case s.Example != "" && len(s.Locals) == 0:
		return NotSetUp
	case s.Example == "":
		return NoExample
	case len(s.Unread) > 0:
		return Unchecked
	}
	return Complete
}

// ID tells sets apart across scans.
func (s Set) ID() string {
	return filepath.Join(s.Dir, s.Example)
}

// Base is the local file the example describes, such as .env for .env.example.
func (s Set) Base() string {
	if s.Example == "" {
		return ""
	}
	return exampleBase(s.Example)
}

// Sort orders sets most urgent first, then by name.
func Sort(sets []Set) {
	slices.SortStableFunc(sets, func(a, b Set) int {
		return cmp.Or(cmp.Compare(a.Attention(), b.Attention()), cmp.Compare(a.Name, b.Name), cmp.Compare(a.Example, b.Example))
	})
}
