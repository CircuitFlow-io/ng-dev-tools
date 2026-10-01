package claudesessions

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Activity is what Claude Code is doing with a session.
type Activity int

const (
	// Closed means no running Claude Code has the session open.
	Closed Activity = iota
	// Idle means Claude has finished its turn and waits for your next prompt.
	Idle
	// Waiting means Claude is blocked on you, such as on a permission prompt.
	Waiting
	// Working means Claude is in the middle of a turn.
	Working
)

var activityNames = map[Activity]string{Closed: "closed", Idle: "idle", Waiting: "waiting", Working: "working"}

func (a Activity) String() string {
	return activityNames[a]
}

// statusActivities maps the status Claude Code writes to its activity.
var statusActivities = map[string]Activity{"idle": Idle, "waiting": Waiting, "busy": Working}

// Live is a session's state in the running Claude Code that has it open.
type Live struct {
	Activity Activity
	// WaitingFor is what a Waiting session needs from you, such as "permission".
	WaitingFor string
	// Since is when the activity last changed.
	Since time.Time
	PID   int
	// Entrypoint is how that Claude Code was started, such as "cli" or "claude-desktop".
	Entrypoint string
}

// busierThan prefers the process doing the most, then the one that changed last, when several
// have the same session open.
func (l Live) busierThan(other Live) bool {
	if l.Activity != other.Activity {
		return l.Activity > other.Activity
	}
	return l.Since.After(other.Since)
}

const (
	liveDirName  = "sessions"
	liveFileExt  = ".json"
	psTimeout    = 2 * time.Second
	psStartField = "pid=,lstart="
)

// liveRecord is the status file a running Claude Code keeps, named after its pid.
type liveRecord struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	ProcStart  string `json:"procStart"`
	Entrypoint string `json:"entrypoint"`
	Status     string `json:"status"`
	WaitingFor string `json:"waitingFor"`
	// StatusUpdatedAt is in milliseconds since the epoch.
	StatusUpdatedAt int64 `json:"statusUpdatedAt"`
	// Spare marks a process started ahead of time that has not taken a session yet.
	Spare bool `json:"spare"`
}

func (r liveRecord) live() Live {
	l := Live{Activity: activityOf(r.Status), WaitingFor: r.WaitingFor, PID: r.PID, Entrypoint: r.Entrypoint}
	if r.StatusUpdatedAt > 0 {
		l.Since = time.UnixMilli(r.StatusUpdatedAt)
	}
	return l
}

// activityOf reads a running process's status. One this version does not know, or none, still
// means the session is open.
func activityOf(status string) Activity {
	if a, ok := statusActivities[status]; ok {
		return a
	}
	return Idle
}

// StartTimes reports when each of pids that is running started, as ps prints it in UTC.
type StartTimes func(ctx context.Context, pids []int) map[int]string

// LiveDir is where running Claude Code processes keep their status: $CLAUDE_CONFIG_DIR/sessions,
// or ~/.claude/sessions.
func LiveDir(home string, getenv func(string) string) string {
	return filepath.Join(configDir(home, getenv), liveDirName)
}

// ReadLive reads the status file of every running Claude Code in dir and returns the state of
// each session one has open, by session id. Files left by a process that has exited, or whose pid
// now belongs to another process, are ignored.
func ReadLive(ctx context.Context, dir string, startTimes StartTimes) map[string]Live {
	records := readLiveRecords(dir)
	if len(records) == 0 {
		return nil
	}
	pids := make([]int, len(records))
	for i, r := range records {
		pids[i] = r.PID
	}
	running := startTimes(ctx, pids)
	sessions := map[string]Live{}
	for _, r := range records {
		started, ok := running[r.PID]
		if !ok || r.ProcStart != "" && oneLine(r.ProcStart) != started {
			continue
		}
		l := r.live()
		if current, seen := sessions[r.SessionID]; !seen || l.busierThan(current) {
			sessions[r.SessionID] = l
		}
	}
	return sessions
}

// readLiveRecords reads the status files in dir, skipping any that cannot be read, such as one
// being written.
func readLiveRecords(dir string) []liveRecord {
	paths, _ := filepath.Glob(filepath.Join(dir, "*"+liveFileExt))
	var records []liveRecord
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var r liveRecord
		if json.Unmarshal(data, &r) != nil || r.PID <= 0 || r.SessionID == "" || r.Spare {
			continue
		}
		records = append(records, r)
	}
	return records
}

// PSStartTimes asks ps when each process started, in UTC and the C locale, the way Claude Code
// records procStart.
func PSStartTimes(ctx context.Context, pids []int) map[int]string {
	ctx, cancel := context.WithTimeout(ctx, psTimeout)
	defer cancel()
	ids := make([]string, 0, len(pids))
	for _, pid := range slices.Compact(slices.Sorted(slices.Values(pids))) {
		ids = append(ids, strconv.Itoa(pid))
	}
	cmd := exec.CommandContext(ctx, "ps", "-o", psStartField, "-p", strings.Join(ids, ","))
	cmd.Env = append(os.Environ(), "LC_ALL=C", "TZ=UTC")
	// ps exits non-zero when any of the pids is not running, and still lists the others.
	out, _ := cmd.Output()
	return parseStartTimes(string(out))
}

// parseStartTimes reads lines of `ps -o pid=,lstart=`, such as "4044 Tue Sep 29 19:18:46 2026".
func parseStartTimes(out string) map[int]string {
	times := map[int]string{}
	for line := range strings.Lines(out) {
		pid, started, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(pid); err == nil {
			times[n] = oneLine(started)
		}
	}
	return times
}
