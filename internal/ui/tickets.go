package ui

import (
	"regexp"

	"charm.land/lipgloss/v2"
)

var ticketKeyPattern = regexp.MustCompile(`[A-Z][A-Z0-9_]+-[0-9]+`)

// ticketURLPrefix is what a ticket key is appended to for its page; "" leaves keys as plain text.
var ticketURLPrefix string

// LinkTickets makes the ticket keys that TicketCell and RenderTickets find, such as TS-1234,
// terminal links to urlPrefix followed by the key. An empty urlPrefix turns linking off.
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
	for _, match := range ticketKeyPattern.FindAllStringIndex(text, -1) {
		start, end := match[0], match[1]
		if start > 0 && isASCIIAlphanumeric(text[start-1]) {
			continue
		}
		if start > written {
			c = append(c, NewSpan(text[written:start], style))
		}
		key := text[start:end]
		c = append(c, NewSpan(key, style.Underline(true).Hyperlink(ticketURLPrefix+key)))
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

func isASCIIAlphanumeric(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}
