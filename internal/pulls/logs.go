package pulls

import (
	"context"
	"errors"
	"regexp"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	// maxLogLines keeps the end of a long log, where the failure is.
	maxLogLines   = 2000
	logFields     = 3
	logExpiredErr = "HTTP 410"
)

var (
	ErrNoLog      = errors.New("only GitHub Actions jobs have a log to show here")
	ErrLogExpired = errors.New("GitHub no longer keeps this log: Actions logs expire after 90 days")

	actionsJobURL = regexp.MustCompile(`^https://github\.com/([^/]+/[^/]+)/actions/runs/\d+/job/(\d+)`)
	logTimestamp  = regexp.MustCompile(`^\d{4}-\d\d-\d\dT[\d:.]+Z ?`)
)

// LogLine is one line of a job's log with the step it belongs to.
type LogLine struct {
	Step string
	Text string
}

// FailedLog reads the log of the failed steps of a GitHub Actions check.
func FailedLog(ctx context.Context, runner macos.Runner, check Check) ([]LogLine, error) {
	m := actionsJobURL.FindStringSubmatch(check.URL)
	if m == nil {
		return nil, ErrNoLog
	}
	repo, job := m[1], m[2]
	out, err := runner.Run(ctx, "gh", "run", "view", "--job", job, "--repo", repo, "--log-failed")
	if err != nil {
		if strings.Contains(err.Error(), logExpiredErr) {
			return nil, ErrLogExpired
		}
		return nil, err
	}
	return parseLog(string(out)), nil
}

// parseLog reads gh's "job<TAB>step<TAB>timestamp text" lines, dropping the job and timestamp
// and any colour codes.
func parseLog(out string) []LogLine {
	var lines []LogLine
	for line := range strings.Lines(out) {
		fields := strings.SplitN(strings.TrimRight(line, "\r\n"), "\t", logFields)
		if len(fields) < logFields {
			continue
		}
		text := logTimestamp.ReplaceAllString(fields[2], "")
		lines = append(lines, LogLine{Step: fields[1], Text: ansi.Strip(text)})
	}
	if len(lines) > maxLogLines {
		lines = lines[len(lines)-maxLogLines:]
	}
	return lines
}
