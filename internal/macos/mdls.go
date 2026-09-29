package macos

import (
	"bytes"
	"context"
	"time"
)

const (
	mdlsNull       = "(null)"
	mdlsDateLayout = "2006-01-02 15:04:05 -0700"
)

// LastUsedDates returns the Spotlight last-used date for each path that has one.
func LastUsedDates(ctx context.Context, r Runner, paths []string) (map[string]time.Time, error) {
	values, err := spotlightAttribute(ctx, r, "kMDItemLastUsedDate", paths)
	if err != nil {
		return nil, err
	}
	dates := make(map[string]time.Time, len(values))
	for path, value := range values {
		if t, err := time.Parse(mdlsDateLayout, value); err == nil {
			dates[path] = t
		}
	}
	return dates, nil
}

// spotlightAttribute reads one metadata attribute for many paths with a single mdls call.
// With -raw, mdls separates the values with NUL bytes in argument order.
func spotlightAttribute(ctx context.Context, r Runner, attribute string, paths []string) (map[string]string, error) {
	if len(paths) == 0 {
		return map[string]string{}, nil
	}
	args := append([]string{"-name", attribute, "-raw"}, paths...)
	out, err := r.Run(ctx, "mdls", args...)
	if err != nil {
		return nil, err
	}
	return parseRawValues(out, paths), nil
}

func parseRawValues(out []byte, paths []string) map[string]string {
	fields := bytes.Split(bytes.TrimSuffix(out, []byte{0}), []byte{0})
	values := make(map[string]string, len(paths))
	for i, field := range fields {
		if i >= len(paths) {
			break
		}
		value := string(field)
		if value == mdlsNull || value == "" {
			continue
		}
		values[paths[i]] = value
	}
	return values
}
