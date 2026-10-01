package ui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
)

const helpSeparator = " · "

var ticketKeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9_]+-[0-9]+`)

// ticketURLPrefix is what a ticket key is appended to for its page; "" leaves keys as plain text.
var ticketURLPrefix string

// LinkTickets makes the ticket keys that TicketCell and RenderTickets find, such as TS-1234,
// terminal links to urlPrefix followed by the key, and lets FindTicket find their pages. An empty
// urlPrefix turns linking off.
func LinkTickets(urlPrefix string) {
	ticketURLPrefix = urlPrefix
}

// TicketCell is text in style, with each ticket key in it a link to the ticket's page.
func TicketCell(text string, style lipgloss.Style) Cell {
	if ticketURLPrefix == "" {
		return Cell{NewSpan(text, style)}
	}
	var c Cell
	written := 0
	for _, key := range ticketKeys(text) {
		start, end := key[0], key[1]
		if start > written {
			c = append(c, NewSpan(text[written:start], style))
		}
		c = append(c, NewSpan(text[start:end], style.Underline(true).Hyperlink(ticketURLPrefix+text[start:end])))
		written = end
	}
	if written < len(text) || len(c) == 0 {
		c = append(c, NewSpan(text[written:], style))
	}
	return c
}

// RenderTickets renders text in style, with each ticket key in it a link to the ticket's page.
func RenderTickets(text string, style lipgloss.Style) string {
	return TicketCell(text, style).String()
}

// Ticket is a ticket key and its page.
type Ticket struct {
	Key string
	URL string
}

// FindTicket is the first ticket key in texts, looked for in order. It lets terminals that cannot
// open links, such as Terminal.app, open the ticket from a key instead.
func FindTicket(texts ...string) (Ticket, bool) {
	if ticketURLPrefix == "" {
		return Ticket{}, false
	}
	for _, text := range texts {
		if keys := ticketKeys(text); len(keys) > 0 {
			key := text[keys[0][0]:keys[0][1]]
			return Ticket{Key: key, URL: ticketURLPrefix + key}, true
		}
	}
	return Ticket{}, false
}

// NoTicketReason says why no ticket opened for the item described by where, such as a branch.
func NoTicketReason(where string) string {
	if ticketURLPrefix == "" {
		return "Set your Jira site with ngt settings set jiraHost <host> to open tickets"
	}
	return "No ticket key in " + where
}

// WithTicketHelp adds "key ticket" to help's key hints, before the last one (quit or back), when
// ticket keys link to a Jira site.
func WithTicketHelp(help, key string) string {
	if ticketURLPrefix == "" {
		return help
	}
	hint := key + " ticket"
	cut := strings.LastIndex(help, helpSeparator)
	if cut < 0 {
		return help + helpSeparator + hint
	}
	return help[:cut] + helpSeparator + hint + help[cut:]
}

// ticketKeys are the start and end of each ticket key in text. A key glued to a letter or digit
// before it, such as the TS-1 in abcTS-1, is not one.
func ticketKeys(text string) [][]int {
	var keys [][]int
	for _, match := range ticketKeyPattern.FindAllStringIndex(text, -1) {
		if match[0] > 0 && isASCIIAlphanumeric(text[match[0]-1]) {
			continue
		}
		keys = append(keys, match)
	}
	return keys
}

func isASCIIAlphanumeric(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
