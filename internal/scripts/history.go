package scripts

import (
	"slices"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/config"
)

const (
	historyFile   = "run.json"
	maxRecentRuns = 5
)

// Run is a script that was run, with its package folder relative to the project root.
type Run struct {
	Package string    `json:"package"`
	Script  string    `json:"script"`
	At      time.Time `json:"at"`
}

// History remembers the latest runs of each project, newest first, keyed by project root.
type History struct {
	Runs map[string][]Run `json:"runs,omitempty"`
}

// Record puts run first in root's runs, dropping an earlier run of the same script.
func (h *History) Record(root string, run Run) {
	if h.Runs == nil {
		h.Runs = map[string][]Run{}
	}
	runs := slices.DeleteFunc(slices.Clone(h.Runs[root]), func(r Run) bool {
		return r.Package == run.Package && r.Script == run.Script
	})
	runs = append([]Run{run}, runs...)
	h.Runs[root] = runs[:min(len(runs), maxRecentRuns)]
}

// Recent is root's runs, newest first.
func (h History) Recent(root string) []Run {
	return h.Runs[root]
}

// LastAnywhere is the newest run over all projects.
func (h History) LastAnywhere() (root string, run Run, ok bool) {
	for r, runs := range h.Runs {
		if len(runs) > 0 && (!ok || runs[0].At.After(run.At)) {
			root, run, ok = r, runs[0], true
		}
	}
	return root, run, ok
}

// LastRun maps each project root to when a script last ran there.
func (h History) LastRun() map[string]time.Time {
	last := map[string]time.Time{}
	for root, runs := range h.Runs {
		if len(runs) > 0 {
			last[root] = runs[0].At
		}
	}
	return last
}

// HistoryStore keeps History in a JSON file.
type HistoryStore struct {
	Path string
}

// DefaultHistoryStore is $XDG_CONFIG_HOME/ngt/run.json, or ~/.config/ngt/run.json.
func DefaultHistoryStore(home string, getenv func(string) string) HistoryStore {
	return HistoryStore{Path: config.Path(home, getenv, historyFile)}
}

// Load reads the history, which is empty before the first save.
func (s HistoryStore) Load() (History, error) {
	var h History
	if err := config.Load(s.Path, &h); err != nil {
		return History{}, err
	}
	return h, nil
}

// Save writes the history.
func (s HistoryStore) Save(h History) error {
	return config.Save(s.Path, h)
}
