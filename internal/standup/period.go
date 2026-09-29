package standup

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const dateOnly = "2006-01-02"

var weekdays = map[string]time.Weekday{
	"sunday": time.Sunday, "monday": time.Monday, "tuesday": time.Tuesday, "wednesday": time.Wednesday,
	"thursday": time.Thursday, "friday": time.Friday, "saturday": time.Saturday,
}

// ParseSince reads when the report starts: "today", "yesterday", a weekday ("monday" or "mon",
// the most recent one, today included), a date (2026-09-28), or days or weeks back ("3d", "2w").
func ParseSince(value string, now time.Time) (time.Time, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	today := StartOfDay(now)
	switch v {
	case "today":
		return today, nil
	case "yesterday":
		return today.AddDate(0, 0, -1), nil
	}
	if weekday, ok := parseWeekday(v); ok {
		back := (int(today.Weekday()) - int(weekday) + len(weekdays)) % len(weekdays)
		return today.AddDate(0, 0, -back), nil
	}
	if date, err := time.ParseInLocation(dateOnly, v, now.Location()); err == nil {
		if date.After(now) {
			return time.Time{}, fmt.Errorf("--since %s is in the future", value)
		}
		return date, nil
	}
	if back, ok := parseDaysBack(v); ok {
		return today.AddDate(0, 0, -back), nil
	}
	return time.Time{}, fmt.Errorf("--since %q: use today, yesterday, a weekday, a date like 2026-09-28, or 3d or 2w", value)
}

func parseWeekday(v string) (time.Weekday, bool) {
	if weekday, ok := weekdays[v]; ok {
		return weekday, true
	}
	for name, weekday := range weekdays {
		if len(v) == 3 && strings.HasPrefix(name, v) {
			return weekday, true
		}
	}
	return 0, false
}

func parseDaysBack(v string) (int, bool) {
	number, unit := strings.TrimRight(v, "dw"), strings.TrimLeft(v, "0123456789")
	n, err := strconv.Atoi(number)
	if err != nil || n < 0 {
		return 0, false
	}
	switch unit {
	case "d":
		return n, true
	case "w":
		return n * len(weekdays), true
	}
	return 0, false
}

// StartOfDay is midnight at the start of t's day, in t's location.
func StartOfDay(t time.Time) time.Time {
	year, month, d := t.Date()
	return time.Date(year, month, d, 0, 0, 0, 0, t.Location())
}

// previousWorkday is the start of the last weekday before now's day: Friday on a Monday or a weekend.
func previousWorkday(now time.Time) time.Time {
	d := StartOfDay(now).AddDate(0, 0, -1)
	for d.Weekday() == time.Saturday || d.Weekday() == time.Sunday {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// lastWorkedDay is the start of the newest day before today among the times you committed, or the
// previous weekday when there is none.
func lastWorkedDay(commitTimes []time.Time, now time.Time) time.Time {
	today := StartOfDay(now)
	var latest time.Time
	for _, t := range commitTimes {
		if t.Before(today) && t.After(latest) {
			latest = t
		}
	}
	if latest.IsZero() {
		return previousWorkday(now)
	}
	return StartOfDay(latest.In(now.Location()))
}
