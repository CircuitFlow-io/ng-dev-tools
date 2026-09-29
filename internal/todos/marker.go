package todos

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Marker is the kind of note left in a comment.
type Marker string

const (
	Todo  Marker = "TODO"
	Fixme Marker = "FIXME"
	Hack  Marker = "HACK"
)

// markerWord finds a marker as a whole word, in capitals, so the word todo in prose is not one.
var markerWord = regexp.MustCompile(`\b(TODO|FIXME|HACK)\b`)

const (
	lineSlash  = "//"
	blockSlash = "/*"
	hash       = "#"
	dashes     = "--"
	html       = "<!--"
	semicolon  = ";"
)

// commentOpeners are the ways a comment starts, by file extension; files with no extension, such
// as Makefile and Fastfile, take #. Others, from Go and TypeScript to Swift, take defaultOpeners.
var (
	defaultOpeners = []string{lineSlash, blockSlash, hash}
	commentOpeners = map[string][]string{
		".py": {hash}, ".rb": {hash}, ".sh": {hash}, ".bash": {hash}, ".zsh": {hash}, ".fish": {hash},
		".yml": {hash}, ".yaml": {hash}, ".toml": {hash}, ".tf": {hash, lineSlash, blockSlash}, ".hcl": {hash, lineSlash, blockSlash},
		".r": {hash}, ".pl": {hash}, ".ex": {hash}, ".exs": {hash}, ".nix": {hash}, ".cmake": {hash}, ".conf": {hash},
		".properties": {hash}, ".ini": {hash, semicolon}, ".php": {lineSlash, blockSlash, hash},
		".sql": {dashes, blockSlash}, ".lua": {dashes}, ".hs": {dashes}, ".elm": {dashes},
		".html": {html}, ".htm": {html}, ".xml": {html}, ".svg": {html},
		".vue": {lineSlash, blockSlash, html}, ".svelte": {lineSlash, blockSlash, html}, ".astro": {lineSlash, blockSlash, html},
		".css": {blockSlash}, ".clj": {semicolon}, ".el": {semicolon}, ".lisp": {semicolon},
		"": {hash},
	}
	markdownExtensions = map[string]bool{".md": true, ".mdx": true, ".markdown": true}
)

// quotes start string literals, in which a comment opener is only text.
const quotes = "\"'`"

// noteTrim is what separates a marker from its note: "TODO: x", "TODO(sam) - x", "**TODO**: x".
const noteTrim = " \t:-*_)>]"

// parseMarker finds the marker a line's comment starts with, and its note. A marker in code, as in
// id: 'TODO', or in the middle of a comment that only mentions it, is not one. Tool directives may
// come first, as in "// @ts-expect-error FIXME". In Markdown the marker must start the line, after
// quote, list or emphasis marks.
func parseMarker(path, line string) (Marker, string, bool) {
	loc := markerWord.FindStringSubmatchIndex(line)
	if loc == nil {
		return "", "", false
	}
	before, marker, after := line[:loc[0]], Marker(line[loc[2]:loc[3]]), line[loc[1]:]
	lead, ok := commentLead(path, before)
	if !ok || !onlyDirectives(lead) {
		return "", "", false
	}
	return marker, note(lead, after), true
}

// commentLead is the comment's text before the marker, and whether the marker is in a comment at all.
func commentLead(path, before string) (string, bool) {
	trimmed := strings.TrimSpace(before)
	ext := strings.ToLower(filepath.Ext(path))
	if markdownExtensions[ext] {
		return "", strings.Trim(trimmed, "> -*_[]") == ""
	}
	openers := openersFor(ext)
	if start, opener, ok := firstOpener(before, openers); ok {
		return before[start+len(opener):], true
	}
	if slices.Contains(openers, blockSlash) && strings.HasPrefix(trimmed, "*") {
		return trimmed, true
	}
	return "", false
}

func openersFor(ext string) []string {
	if openers, ok := commentOpeners[ext]; ok {
		return openers
	}
	return defaultOpeners
}

// firstOpener finds where the comment starts: the first opener outside a string literal, so a
// URL or a "// TODO" in a string is not one.
func firstOpener(text string, openers []string) (int, string, bool) {
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0 && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case strings.IndexByte(quotes, c) >= 0:
			quote = c
		default:
			if opener, ok := openerAt(text[i:], openers); ok {
				return i, opener, true
			}
		}
	}
	return 0, "", false
}

func openerAt(text string, openers []string) (string, bool) {
	for _, opener := range openers {
		if strings.HasPrefix(text, opener) {
			return opener, true
		}
	}
	return "", false
}

// onlyDirectives reports whether the comment text before a marker is empty or only tool
// directives such as @ts-expect-error.
func onlyDirectives(lead string) bool {
	for _, word := range strings.Fields(strings.Trim(lead, " \t*!")) {
		if !strings.HasPrefix(word, "@") {
			return false
		}
	}
	return true
}

// note is the text after the marker, or the directives before it when the marker ends the comment.
func note(lead, after string) string {
	text := strings.TrimLeft(skipOwner(after), noteTrim)
	if text == "" {
		text = strings.Trim(lead, " \t*!")
	}
	return strings.TrimSpace(trimCommentClose(text))
}

// skipOwner drops the "(sam)" of "TODO(sam): x".
func skipOwner(after string) string {
	if !strings.HasPrefix(after, "(") {
		return after
	}
	if _, rest, ok := strings.Cut(after, ")"); ok {
		return rest
	}
	return after
}

func trimCommentClose(text string) string {
	for _, closer := range []string{"*/}", "*/", "-->", "**"} {
		text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), closer))
	}
	return text
}
