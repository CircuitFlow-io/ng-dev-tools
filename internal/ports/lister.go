package ports

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

// lsofNoMatchesExit is lsof's exit status when nothing matches the selection.
const lsofNoMatchesExit = 1

// Lister finds the processes listening on TCP ports that the current user can see.
type Lister struct {
	Runner macos.Runner
	Home   string
}

// List returns listening processes sorted by their lowest port.
func (l Lister) List(ctx context.Context) ([]Process, error) {
	out, err := l.Runner.Run(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN", "-Fpcn")
	if err != nil && !isNoMatches(err, out) {
		return nil, fmt.Errorf("listing listening sockets: %w", err)
	}
	processes := parseListeners(string(out))
	if len(processes) == 0 {
		return nil, nil
	}
	l.describe(ctx, processes, time.Now())
	slices.SortFunc(processes, func(a, b Process) int {
		return cmp.Or(cmp.Compare(a.Ports[0], b.Ports[0]), cmp.Compare(a.PID, b.PID))
	})
	return processes, nil
}

func isNoMatches(err error, out []byte) bool {
	var exitErr *exec.ExitError
	return len(out) == 0 && errors.As(err, &exitErr) && exitErr.ExitCode() == lsofNoMatchesExit
}

// describe fills in what lsof's socket listing does not include. Each detail is best effort.
func (l Lister) describe(ctx context.Context, processes []Process, now time.Time) {
	pids := pidList(processes)
	infos := parseProcessInfo(l.output(ctx, "ps", "-ww", "-o", "pid=,etime=,comm=", "-p", pids))
	commandLines := parseCommandLines(l.output(ctx, "ps", "-ww", "-o", "pid=,command=", "-p", pids))
	workDirs := parseWorkDirs(l.output(ctx, "lsof", "-a", "-nP", "-d", "cwd", "-Fpn", "-p", pids))

	for i := range processes {
		p := &processes[i]
		if info, ok := infos[p.PID]; ok {
			p.Executable = info.executable
			p.StartedAt = now.Add(-info.elapsed)
		}
		p.CommandLine = commandLines[p.PID]
		p.WorkDir = workDirs[p.PID]
		p.Project = projectRoot(p.WorkDir, l.Home)
	}
}

// output ignores failures because ps and lsof exit non-zero when any listed process has
// exited in the meantime, while still reporting the others.
func (l Lister) output(ctx context.Context, name string, args ...string) string {
	out, _ := l.Runner.Run(ctx, name, args...)
	return string(out)
}

func pidList(processes []Process) string {
	pids := make([]string, len(processes))
	for i, p := range processes {
		pids[i] = strconv.Itoa(p.PID)
	}
	return strings.Join(pids, ",")
}

// projectRoot is the git repository containing workDir, or workDir itself. Apps started by
// launchd run in "/", which says nothing about them, so it maps to "".
func projectRoot(workDir, home string) string {
	if workDir == "" || workDir == "/" {
		return ""
	}
	for dir := workDir; dir != home && dir != "/"; dir = filepath.Dir(dir) {
		if fsx.Exists(filepath.Join(dir, ".git")) {
			return dir
		}
	}
	return workDir
}
