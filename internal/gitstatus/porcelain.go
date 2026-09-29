package gitstatus

import (
	"strconv"
	"strings"
)

const (
	unbornOID      = "(initial)"
	detachedHead   = "(detached)"
	shortHashWidth = 7
	unchanged      = '.'
	// The number of space-separated fields before the path in porcelain v2 entries.
	changedFields   = 8
	renamedFields   = 9
	unmergedFields  = 10
	untrackedPrefix = "? "
)

// FileKind says how git lists a changed file.
type FileKind int

const (
	Changed FileKind = iota
	Unmerged
	Untracked
)

// File is one entry of git status. Staged and Unstaged are the X and Y status letters, with '.'
// for no change on that side.
type File struct {
	Path     string
	From     string
	Staged   byte
	Unstaged byte
	Kind     FileKind
}

// Code is the two-letter status as `git status --short` shows it, such as "M ", " M" or "??".
func (f File) Code() string {
	if f.Kind == Untracked {
		return "??"
	}
	return string([]byte{blankIfUnchanged(f.Staged), blankIfUnchanged(f.Unstaged)})
}

func blankIfUnchanged(letter byte) byte {
	if letter == unchanged {
		return ' '
	}
	return letter
}

// Changes counts the changed files by kind. A file staged and then changed again counts as both.
type Changes struct {
	Staged     int
	Modified   int
	Untracked  int
	Conflicted int
}

// Any reports whether there is anything uncommitted.
func (c Changes) Any() bool {
	return c != Changes{}
}

// Changes counts r's changed files.
func (r Repo) Changes() Changes {
	var c Changes
	for _, f := range r.Files {
		switch f.Kind {
		case Untracked:
			c.Untracked++
		case Unmerged:
			c.Conflicted++
		default:
			if f.Staged != unchanged {
				c.Staged++
			}
			if f.Unstaged != unchanged {
				c.Modified++
			}
		}
	}
	return c
}

// status is what `git status --porcelain=v2 --branch -z` reports.
type status struct {
	oid      string
	head     string
	upstream string
	hasAB    bool
	ahead    int
	behind   int
	files    []File
}

func parseStatus(out []byte) status {
	var s status
	entries := strings.Split(string(out), "\x00")
	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		switch {
		case strings.HasPrefix(entry, "# "):
			s.applyHeader(strings.TrimPrefix(entry, "# "))
		case strings.HasPrefix(entry, "1 "):
			s.addFile(entry, changedFields, Changed)
		case strings.HasPrefix(entry, "2 "):
			s.addFile(entry, renamedFields, Changed)
			if i+1 < len(entries) {
				i++
				s.files[len(s.files)-1].From = entries[i]
			}
		case strings.HasPrefix(entry, "u "):
			s.addFile(entry, unmergedFields, Unmerged)
		case strings.HasPrefix(entry, untrackedPrefix):
			s.files = append(s.files, File{Path: strings.TrimPrefix(entry, untrackedPrefix), Kind: Untracked})
		}
	}
	return s
}

func (s *status) applyHeader(header string) {
	key, value, _ := strings.Cut(header, " ")
	switch key {
	case "branch.oid":
		s.oid = value
	case "branch.head":
		s.head = value
	case "branch.upstream":
		s.upstream = value
	case "branch.ab":
		s.hasAB = true
		ahead, behind, _ := strings.Cut(value, " ")
		s.ahead, _ = strconv.Atoi(strings.TrimPrefix(ahead, "+"))
		s.behind, _ = strconv.Atoi(strings.TrimPrefix(behind, "-"))
	}
}

// addFile parses an entry whose path follows fields space-separated fields, the first being the
// entry type and the second the XY letters.
func (s *status) addFile(entry string, fields int, kind FileKind) {
	parts := strings.SplitN(entry, " ", fields+1)
	if len(parts) <= fields || len(parts[1]) != 2 {
		return
	}
	s.files = append(s.files, File{Path: parts[fields], Staged: parts[1][0], Unstaged: parts[1][1], Kind: kind})
}

func shortHash(oid string) string {
	return oid[:min(len(oid), shortHashWidth)]
}
