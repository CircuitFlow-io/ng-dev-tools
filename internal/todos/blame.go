package todos

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

// uncommittedHash is what git blame gives a line that is not committed yet.
const uncommittedHash = "0000000000000000000000000000000000000000"

const shortHashLength = 7

// blame is who last changed a line, and when.
type blame struct {
	hash    string
	author  string
	email   string
	at      time.Time
	subject string
}

// blameFile dates the items at indexes, all in file, with one git blame of their lines. A file
// git cannot blame, such as an untracked one, leaves its items uncommitted.
func blameFile(ctx context.Context, runner macos.Runner, dir, file string, items []Item, indexes []int) {
	args := []string{"blame", "--line-porcelain", "-w"}
	for _, i := range indexes {
		n := strconv.Itoa(items[i].Line)
		args = append(args, "-L", n+","+n)
	}
	out, err := git(ctx, runner, dir, append(args, "--", file)...)
	blames := map[int]blame{}
	if err == nil {
		blames = parseBlame(string(out))
	}
	for _, i := range indexes {
		b, ok := blames[items[i].Line]
		if !ok || b.hash == uncommittedHash {
			items[i].Uncommitted = true
			continue
		}
		items[i].Commit, items[i].Author, items[i].Email, items[i].At, items[i].Subject = b.hash, b.author, b.email, b.at, b.subject
	}
}

// parseBlame reads git blame --line-porcelain output into each final line's blame.
func parseBlame(out string) map[int]blame {
	blames := map[int]blame{}
	var current blame
	line := 0
	for text := range strings.Lines(out) {
		text = strings.TrimSuffix(text, "\n")
		if strings.HasPrefix(text, "\t") {
			blames[line] = current
			continue
		}
		key, value, _ := strings.Cut(text, " ")
		switch key {
		case "author":
			current.author = value
		case "author-mail":
			current.email = strings.Trim(value, "<>")
		case "author-time":
			current.at = unixTime(value)
		case "summary":
			current.subject = value
		default:
			if isHash(key) {
				current = blame{hash: key}
				line = finalLine(value)
			}
		}
	}
	return blames
}

// finalLine reads the line number in the file from "<original> <final> [<count>]".
func finalLine(header string) int {
	fields := strings.Fields(header)
	if len(fields) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(fields[1])
	return n
}

func isHash(s string) bool {
	if len(s) != len(uncommittedHash) {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func unixTime(s string) time.Time {
	seconds, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(seconds, 0)
}

// ShortCommit is the commit's abbreviated hash.
func (i Item) ShortCommit() string {
	return i.Commit[:min(len(i.Commit), shortHashLength)]
}
