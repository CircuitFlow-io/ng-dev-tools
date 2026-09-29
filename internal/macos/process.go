package macos

import (
	"context"
	"strings"
)

// RunningExecutables returns the executable path of every running process.
func RunningExecutables(ctx context.Context, r Runner) ([]string, error) {
	out, err := r.Run(ctx, "ps", "-axo", "comm=")
	if err != nil {
		return nil, err
	}
	var paths []string
	for line := range strings.Lines(string(out)) {
		if path := strings.TrimSpace(line); path != "" {
			paths = append(paths, path)
		}
	}
	return paths, nil
}
