package ui

import (
	"reflect"
	"strings"
	"testing"
)

const testTicketPrefix = "https://acme.atlassian.net/browse/"

func linkTicketsForTest(t *testing.T) {
	t.Helper()
	LinkTickets(testTicketPrefix)
	t.Cleanup(func() { LinkTickets("") })
}

func links(c Cell) map[string]string {
	found := map[string]string{}
	for _, s := range c {
		if link, _ := s.style.GetHyperlink(); link != "" {
			found[s.text] = link
		}
	}
	return found
}

func TestTicketCellLinksTicketKeys(t *testing.T) {
	linkTicketsForTest(t)
	tests := []struct {
		text string
		want []string
	}{
		{"feat/TS-234455-login-form", []string{"TS-234455"}},
		{"COREX-344: fix the retry, see TS-1", []string{"COREX-344", "TS-1"}},
		{"feature_AB2-77_rename", []string{"AB2-77"}},
		{"bugfix/abcTS-12 and x1TS-5", nil},
		{"lowercase ts-123 and single T-1", nil},
		{"", nil},
	}
	for _, tt := range tests {
		c := TicketCell(tt.text, plain)
		if c.Text() != tt.text {
			t.Errorf("TicketCell(%q) text = %q", tt.text, c.Text())
		}
		want := map[string]string{}
		for _, key := range tt.want {
			want[key] = testTicketPrefix + key
		}
		if got := links(c); !reflect.DeepEqual(got, want) {
			t.Errorf("TicketCell(%q) links = %v, want %v", tt.text, got, want)
		}
	}
}

func TestTicketCellIsPlainWithoutJiraHost(t *testing.T) {
	c := TicketCell("feat/TS-1-login", Muted)
	if len(c) != 1 || len(links(c)) != 0 {
		t.Errorf("TicketCell without a host = %#v, want one plain span", c)
	}
}

func TestTicketLinkSurvivesTruncation(t *testing.T) {
	linkTicketsForTest(t)
	rendered := TicketCell("TS-234455 add the login form", plain).Truncate(12).String()
	if !strings.Contains(rendered, "\x1b]8;;"+testTicketPrefix+"TS-234455") {
		t.Errorf("truncated cell lost its link: %q", rendered)
	}
}

func TestFindTicketTakesTheFirstKeyInOrder(t *testing.T) {
	linkTicketsForTest(t)
	ticket, ok := FindTicket("Fix the retry", "feat/COREX-344-retry", "TS-1")
	if !ok || ticket != (Ticket{Key: "COREX-344", URL: testTicketPrefix + "COREX-344"}) {
		t.Errorf("FindTicket = %+v, %v", ticket, ok)
	}
	if _, ok := FindTicket("main", "no key here"); ok {
		t.Error("FindTicket found a key in text without one")
	}
}

func TestFindTicketNeedsAJiraHost(t *testing.T) {
	if _, ok := FindTicket("feat/TS-1-login"); ok {
		t.Error("FindTicket found a ticket without a Jira host")
	}
	if reason := NoTicketReason("main"); !strings.Contains(reason, "jiraHost") {
		t.Errorf("NoTicketReason without a host = %q, want a hint to set jiraHost", reason)
	}
}

func TestWithTicketHelpGoesBeforeTheLastHint(t *testing.T) {
	const help = "↑/↓ move · r refresh · q quit"
	if got := WithTicketHelp(help, "t"); got != help {
		t.Errorf("without a Jira host = %q", got)
	}
	linkTicketsForTest(t)
	if got, want := WithTicketHelp(help, "t"), "↑/↓ move · r refresh · t ticket · q quit"; got != want {
		t.Errorf("WithTicketHelp = %q, want %q", got, want)
	}
}
