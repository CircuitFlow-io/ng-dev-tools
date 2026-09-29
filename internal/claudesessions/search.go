package claudesessions

import (
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	// snippetLead is how many characters of a matching turn are kept before the match.
	snippetLead = 40
	// snippetTail is how many bytes are kept after it, more than any details box shows.
	snippetTail = 400
	ellipsis    = "…"
)

// Index searches sessions by everything said in them, as well as their title, folder, branches
// and models. Letters match in either case.
type Index struct {
	entries []entry
}

type entry struct {
	session Session
	// text is the metadata and then each turn, lowered, one per line. turnStarts holds where each
	// turn begins, and lowering keeps every byte in place, so an offset in text is one in a turn.
	text       string
	turnStarts []int
}

// Result is a session that matches a search, with the turn it matched in.
type Result struct {
	Session Session
	Snippet Snippet
}

// Snippet is a match in a prompt or reply with the text around it. It is empty when only the
// session's title, folder, branches or models matched.
type Snippet struct {
	Yours  bool
	Before string
	Match  string
	After  string
}

// IsZero reports whether there is no snippet.
func (s Snippet) IsZero() bool {
	return s.Match == ""
}

// NewIndex prepares sessions for searching, keeping their order.
func NewIndex(sessions []Session) Index {
	entries := make([]entry, len(sessions))
	for i, s := range sessions {
		entries[i] = newEntry(s)
	}
	return Index{entries: entries}
}

func newEntry(s Session) entry {
	var b strings.Builder
	for _, field := range metadata(s) {
		b.WriteString(field)
		b.WriteByte(' ')
	}
	starts := make([]int, len(s.Turns))
	for i, t := range s.Turns {
		b.WriteByte('\n')
		starts[i] = b.Len()
		b.WriteString(t.Text)
	}
	return entry{session: s, text: lowerASCII(b.String()), turnStarts: starts}
}

func metadata(s Session) []string {
	fields := []string{s.ID, s.Title, s.Dir}
	fields = append(fields, s.Branches...)
	for _, m := range s.Models {
		fields = append(fields, m.ID, ModelName(m.ID))
	}
	return fields
}

// Len is the number of sessions indexed.
func (x Index) Len() int {
	return len(x.entries)
}

// Search returns the sessions in which every word of query appears, in their original order.
// An empty query returns them all.
func (x Index) Search(query string) []Result {
	words := strings.Fields(lowerASCII(query))
	results := make([]Result, 0, len(x.entries))
	for _, e := range x.entries {
		if e.matchesAll(words) {
			results = append(results, Result{Session: e.session, Snippet: e.snippet(words)})
		}
	}
	return results
}

func (e entry) matchesAll(words []string) bool {
	for _, w := range words {
		if !strings.Contains(e.text, w) {
			return false
		}
	}
	return true
}

// wordFit is how a word must sit in the text to count as a match for the snippet.
type wordFit int

const (
	wholeWord wordFit = iota
	wordStart
	anywhere
)

// snippet shows where the query appears in the turns, preferring the whole query to a single
// word, and a whole word to the start of one or to a match inside one.
func (e entry) snippet(words []string) Snippet {
	if len(e.turnStarts) == 0 || len(words) == 0 {
		return Snippet{}
	}
	candidates := words
	if len(words) > 1 {
		candidates = slices.Concat([]string{strings.Join(words, " ")}, words)
	}
	turnsFrom := e.turnStarts[0]
	for _, fit := range []wordFit{wholeWord, wordStart, anywhere} {
		for _, w := range candidates {
			if at := indexFitting(e.text[turnsFrom:], w, fit); at >= 0 {
				return e.snippetAt(turnsFrom+at, len(w))
			}
		}
	}
	return Snippet{}
}

// snippetAt cuts the turn holding text[at:at+size] around it.
func (e entry) snippetAt(at, size int) Snippet {
	turn, exact := slices.BinarySearch(e.turnStarts, at)
	if !exact {
		turn--
	}
	t := e.session.Turns[turn]
	offset := at - e.turnStarts[turn]
	return Snippet{
		Yours:  t.Yours,
		Before: lead(t.Text[:offset]),
		Match:  t.Text[offset : offset+size],
		After:  tail(t.Text[offset+size:]),
	}
}

// indexFitting finds the first place word sits in text as fit asks.
func indexFitting(text, word string, fit wordFit) int {
	for from := 0; ; {
		at := strings.Index(text[from:], word)
		if at < 0 {
			return -1
		}
		at += from
		if fits(text, at, at+len(word), fit) {
			return at
		}
		from = at + 1
	}
}

func fits(text string, start, end int, fit wordFit) bool {
	startsWord := start == 0 || !isWordByte(text[start-1])
	endsWord := end == len(text) || !isWordByte(text[end])
	switch fit {
	case wholeWord:
		return startsWord && endsWord
	case wordStart:
		return startsWord
	default:
		return true
	}
}

func isWordByte(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '_'
}

// lead keeps the end of text, cut at a word where it can be.
func lead(text string) string {
	if utf8.RuneCountInString(text) <= snippetLead {
		return text
	}
	runes := []rune(text)
	kept := string(runes[len(runes)-snippetLead:])
	if space := strings.IndexByte(kept, ' '); space >= 0 && space < len(kept)-1 {
		kept = kept[space+1:]
	}
	return ellipsis + kept
}

func tail(text string) string {
	if len(text) <= snippetTail {
		return text
	}
	cut := snippetTail
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut]
}

// lowerASCII lowers ASCII letters only, so every byte keeps its position.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}
