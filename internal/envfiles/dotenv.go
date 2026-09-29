package envfiles

import (
	"bufio"
	"io"
	"regexp"
	"strings"
)

const maxLineBytes = 1 << 20

var keyName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]*$`)

// Key is one variable of an env file. Its value is never kept: only whether it is empty.
type Key struct {
	Name  string
	Empty bool
	// Comments are the comment lines right above the key, which describe it in an example.
	Comments []string
}

// parseKeys reads the keys of an env file in order. A key set twice keeps its first place and
// its last emptiness, as the last value wins.
func parseKeys(r io.Reader) ([]Key, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, bufio.MaxScanTokenSize), maxLineBytes)
	var keys []Key
	index := map[string]int{}
	var comments []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			comments = nil
			continue
		}
		if strings.HasPrefix(line, "#") {
			comments = append(comments, line)
			continue
		}
		name, value, ok := splitAssignment(line)
		if !ok {
			comments = nil
			continue
		}
		empty := isEmptyValue(value)
		if quote, open := openQuote(value); open {
			skipToClosingQuote(scanner, quote)
		}
		if i, seen := index[name]; seen {
			keys[i].Empty = empty
		} else {
			index[name] = len(keys)
			keys = append(keys, Key{Name: name, Empty: empty, Comments: comments})
		}
		comments = nil
	}
	return keys, scanner.Err()
}

func splitAssignment(line string) (name, value string, ok bool) {
	line = strings.TrimPrefix(line, "export ")
	name, value, ok = strings.Cut(line, "=")
	name = strings.TrimSpace(name)
	if !ok || !keyName.MatchString(name) {
		return "", "", false
	}
	return name, value, true
}

// isEmptyValue reports whether a value is blank, a pair of empty quotes, or only a comment.
func isEmptyValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	switch trimmed {
	case "", `""`, "''", "``":
		return true
	}
	return strings.HasPrefix(trimmed, "#")
}

// openQuote reports whether value starts a quoted value that goes on over the next lines.
func openQuote(value string) (byte, bool) {
	value = strings.TrimSpace(value)
	if value == "" || !strings.ContainsRune(`"'`+"`", rune(value[0])) {
		return 0, false
	}
	quote := value[0]
	return quote, !hasClosingQuote(value[1:], quote)
}

func hasClosingQuote(text string, quote byte) bool {
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '\\':
			i++
		case quote:
			return true
		}
	}
	return false
}

func skipToClosingQuote(scanner *bufio.Scanner, quote byte) {
	for scanner.Scan() {
		if hasClosingQuote(scanner.Text(), quote) {
			return
		}
	}
}
