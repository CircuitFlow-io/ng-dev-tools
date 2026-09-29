package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/dustin/go-humanize"
)

const (
	day   = 24 * time.Hour
	month = 30 * day
	year  = 365 * day
)

// Bytes formats a size like "1.2 GB".
func Bytes(n int64) string {
	if n < 0 {
		n = 0
	}
	return humanize.Bytes(uint64(n))
}

// Age formats how long ago t was, coarsely: "3 days", "5 months", "2 years".
func Age(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := now.Sub(t)
	switch {
	case d >= year:
		return Count(int(d/year), "year")
	case d >= month:
		return Count(int(d/month), "month")
	default:
		return Count(int(d/day), "day")
	}
}

// Ago formats how long ago t was: precisely within the last day ("3h 5m ago"), coarsely before ("5 months ago").
func Ago(now, t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	if d := now.Sub(t); d < day {
		return Elapsed(max(d, time.Second)) + " ago"
	}
	return Age(now, t) + " ago"
}

// Count formats n with a regularly pluralised noun: "1 item", "3 items", "2 processes", "4 entries".
func Count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %s", n, plural(noun))
}

func plural(noun string) string {
	if stem, ok := strings.CutSuffix(noun, "y"); ok && stem != "" && !strings.ContainsAny(stem[len(stem)-1:], "aeiou") {
		return stem + "ies"
	}
	for _, suffix := range []string{"s", "x", "ch", "sh"} {
		if strings.HasSuffix(noun, suffix) {
			return noun + "es"
		}
	}
	return noun + "s"
}

// Elapsed formats a running time compactly: "45s", "12m", "3h 5m", "2 days".
func Elapsed(d time.Duration) string {
	switch {
	case d <= 0:
		return "?"
	case d >= day:
		return Count(int(d/day), "day")
	case d >= time.Hour:
		return fmt.Sprintf("%dh %dm", int(d/time.Hour), int(d%time.Hour/time.Minute))
	case d >= time.Minute:
		return fmt.Sprintf("%dm", int(d/time.Minute))
	default:
		return fmt.Sprintf("%ds", int(d/time.Second))
	}
}
