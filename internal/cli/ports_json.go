package cli

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
)

var stopOutcomeNames = map[ports.Outcome]string{
	ports.Terminated:    "terminated",
	ports.Killed:        "killed",
	ports.AlreadyExited: "already-exited",
}

type portsJSON struct {
	Processes []processJSON `json:"processes"`
}

type processJSON struct {
	PID         int       `json:"pid"`
	Name        string    `json:"name"`
	Ports       []int     `json:"ports"`
	Addresses   []string  `json:"addresses"`
	Executable  string    `json:"executable,omitempty"`
	CommandLine string    `json:"commandLine,omitempty"`
	StartedAt   time.Time `json:"startedAt,omitzero"`
	UptimeMs    int64     `json:"uptimeMs,omitempty"`
	WorkDir     string    `json:"workDir,omitempty"`
	// Project is the git repository the process was started from, or its folder outside one.
	Project string `json:"project,omitempty"`
	// Tag and Caution say why stopping the process may not be what you want, such as "docker".
	Tag     string `json:"tag,omitempty"`
	Caution string `json:"caution,omitempty"`
	System  bool   `json:"system"`
}

type stopJSON struct {
	// Free are the ports nothing was listening on.
	Free    []int            `json:"free"`
	Results []stopResultJSON `json:"results"`
}

type stopResultJSON struct {
	Process processJSON `json:"process"`
	Outcome string      `json:"outcome,omitempty"`
	Error   string      `json:"error,omitempty"`
}

func toPortsJSON(processes []ports.Process, now time.Time) portsJSON {
	views := make([]processJSON, 0, len(processes))
	for _, p := range processes {
		views = append(views, toProcessJSON(p, now))
	}
	return portsJSON{Processes: views}
}

func toProcessJSON(p ports.Process, now time.Time) processJSON {
	view := processJSON{
		PID:         p.PID,
		Name:        p.Name,
		Ports:       p.Ports,
		Addresses:   p.Addresses,
		Executable:  p.Executable,
		CommandLine: p.CommandLine,
		StartedAt:   p.StartedAt,
		WorkDir:     p.WorkDir,
		Project:     p.Project,
		Tag:         p.Tag(),
		Caution:     p.Caution(),
		System:      p.IsSystem(),
	}
	if !p.StartedAt.IsZero() {
		view.UptimeMs = p.Uptime(now).Milliseconds()
	}
	return view
}

func toStopJSON(free []int, results []ports.StopResult, now time.Time) stopJSON {
	views := make([]stopResultJSON, 0, len(results))
	for _, r := range results {
		view := stopResultJSON{Process: toProcessJSON(r.Process, now), Error: errText(r.Err)}
		if r.Err == nil {
			view.Outcome = stopOutcomeNames[r.Outcome]
		}
		views = append(views, view)
	}
	return stopJSON{Free: nonNil(free), Results: views}
}
