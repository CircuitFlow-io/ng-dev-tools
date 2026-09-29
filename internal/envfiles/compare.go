package envfiles

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
)

// ErrNotRegular is returned for an env file that is a named pipe or socket rather than a file.
var ErrNotRegular = errors.New("not a regular file, so it is not read")

// compare fills in the set's missing, extra and empty keys, and adds every key it reads to known.
func compare(s *Set, known map[string]bool) {
	example, err := readExample(s)
	if err != nil {
		s.Err = err
		return
	}
	s.ExampleKeys = len(example)
	listed := map[string]bool{}
	for _, k := range example {
		listed[k.Name] = true
		known[k.Name] = true
	}
	p := newPresence()
	for _, file := range slices.Concat(s.Locals, s.Defaults) {
		if !isRegularFile(filepath.Join(s.Dir, file)) {
			s.Unread = append(s.Unread, file)
			continue
		}
		keys, err := readKeys(s.Dir, file)
		if err != nil {
			s.Err = err
			return
		}
		p.add(keys, file, known)
		if s.Example != "" && isLocal(file) {
			s.Extra = append(s.Extra, unlisted(keys, listed, s.Extra, file)...)
		}
	}
	if len(s.Unread) > 0 {
		return
	}
	s.Empty = p.empty()
	if len(s.Locals) > 0 {
		s.Missing = p.missing(example)
	}
}

// presence is which keys the local files set, and where the ones never set are empty.
type presence struct {
	set        map[string]bool
	emptyIn    map[string]string
	emptyOrder []string
}

func newPresence() presence {
	return presence{set: map[string]bool{}, emptyIn: map[string]string{}}
}

func (p *presence) add(keys []Key, file string, known map[string]bool) {
	for _, k := range keys {
		known[k.Name] = true
		if !k.Empty {
			p.set[k.Name] = true
			continue
		}
		if _, seen := p.emptyIn[k.Name]; !seen {
			p.emptyIn[k.Name] = file
			p.emptyOrder = append(p.emptyOrder, k.Name)
		}
	}
}

// empty are the keys that are empty wherever they appear, with the first file they are in.
func (p presence) empty() []KeyInFile {
	var empty []KeyInFile
	for _, key := range p.emptyOrder {
		if !p.set[key] {
			empty = append(empty, KeyInFile{Key: key, File: p.emptyIn[key]})
		}
	}
	return empty
}

// missing are the example's keys that no file has at all, empty or not.
func (p presence) missing(example []Key) []string {
	var missing []string
	for _, k := range example {
		if !p.set[k.Name] && p.emptyIn[k.Name] == "" {
			missing = append(missing, k.Name)
		}
	}
	return missing
}

func readExample(s *Set) ([]Key, error) {
	if s.Example == "" {
		return nil, nil
	}
	return readKeys(s.Dir, s.Example)
}

// isRegularFile tells a file apart from a named pipe or socket without opening it: opening a pipe
// waits for, or wakes up, whatever serves it.
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func readKeys(dir, file string) ([]Key, error) {
	path := filepath.Join(dir, file)
	if !isRegularFile(path) {
		return nil, fmt.Errorf("%s: %w", file, ErrNotRegular)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	keys, err := parseKeys(f)
	if err != nil {
		return nil, fmt.Errorf("could not read %s: %w", file, err)
	}
	return keys, nil
}

func isLocal(file string) bool {
	role, _ := classify(file)
	return role == Local
}

// unlisted are the keys the example does not list and that no earlier file already reported.
func unlisted(keys []Key, listed map[string]bool, reported []KeyInFile, file string) []KeyInFile {
	seen := map[string]bool{}
	for _, r := range reported {
		seen[r.Key] = true
	}
	var extra []KeyInFile
	for _, k := range keys {
		if !listed[k.Name] && !seen[k.Name] {
			extra = append(extra, KeyInFile{Key: k.Name, File: file})
		}
	}
	return extra
}
