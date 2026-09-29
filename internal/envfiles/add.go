package envfiles

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrNoLocalFile is returned when a set has no local file to add keys to.
var ErrNoLocalFile = errors.New("there is no local env file to add the keys to yet")

// AddTarget is the local file missing keys go to: the one the example is named after, else the
// first local file, leaving out the ones that are not regular files.
func (s Set) AddTarget() (string, bool) {
	readable := slices.DeleteFunc(slices.Clone(s.Locals), func(local string) bool { return slices.Contains(s.Unread, local) })
	if len(readable) == 0 {
		return "", false
	}
	if slices.Contains(readable, s.Base()) {
		return s.Base(), true
	}
	return readable[0], true
}

// AddMissing appends the keys the example lists and the local files lack to the target file, with
// empty values and the comments the example has above them. It reads the files again first, so it
// never adds a key that is there by now, and it never changes a line already in the file. It
// returns the file and how many keys it added.
func AddMissing(s Set) (string, int, error) {
	target, ok := s.AddTarget()
	if !ok || s.Example == "" {
		return "", 0, ErrNoLocalFile
	}
	fresh := Set{Dir: s.Dir, Example: s.Example, Locals: s.Locals, Defaults: s.Defaults}
	compare(&fresh, map[string]bool{})
	if fresh.Err != nil {
		return target, 0, fresh.Err
	}
	if len(fresh.Missing) == 0 {
		return target, 0, nil
	}
	example, err := readKeys(s.Dir, s.Example)
	if err != nil {
		return target, 0, err
	}
	block := missingBlock(example, fresh.Missing)
	return target, len(fresh.Missing), appendBlock(filepath.Join(s.Dir, target), block)
}

func missingBlock(example []Key, missing []string) string {
	wanted := map[string]bool{}
	for _, key := range missing {
		wanted[key] = true
	}
	var b strings.Builder
	for _, k := range example {
		if !wanted[k.Name] {
			continue
		}
		for _, comment := range k.Comments {
			b.WriteString(comment + "\n")
		}
		b.WriteString(k.Name + "=\n")
	}
	return b.String()
}

// appendBlock adds block after a blank line, first ending the file's last line if it is unfinished.
func appendBlock(path, block string) error {
	if !isRegularFile(path) {
		return fmt.Errorf("%s: %w", filepath.Base(path), ErrNotRegular)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	separator, err := separatorFor(f)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(separator + block); err != nil {
		return err
	}
	return f.Close()
}

func separatorFor(f *os.File) (string, error) {
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return "", err
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, info.Size()-1); err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if last[0] == '\n' {
		return "\n", nil
	}
	return "\n\n", nil
}
