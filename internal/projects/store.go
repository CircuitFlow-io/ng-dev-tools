package projects

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/config"
)

const stateFileName = "open.json"

// State is what ngt open remembers between runs.
type State struct {
	// IDE is the app path of the IDE picked last, preselected for projects without one of their own.
	IDE string `json:"ide,omitempty"`
	// ProjectIDEs maps project paths to the app path of the IDE each was last opened in.
	ProjectIDEs map[string]string `json:"projectIDEs,omitempty"`
	// Opened maps project paths to when they were last opened.
	Opened map[string]time.Time `json:"opened,omitempty"`
}

// RecordOpen remembers that project was opened in the IDE at ideAppPath.
func (s *State) RecordOpen(project, ideAppPath string, at time.Time) {
	s.IDE = ideAppPath
	if s.ProjectIDEs == nil {
		s.ProjectIDEs = map[string]string{}
	}
	s.ProjectIDEs[project] = ideAppPath
	if s.Opened == nil {
		s.Opened = map[string]time.Time{}
	}
	s.Opened[project] = at
}

// Store keeps State in a JSON file.
type Store struct {
	Path string
}

// DefaultStore is $XDG_CONFIG_HOME/ngt/open.json, or ~/.config/ngt/open.json.
func DefaultStore(home string, getenv func(string) string) Store {
	return Store{Path: config.Path(home, getenv, stateFileName)}
}

// Load reads the state, which is empty before the first save.
func (s Store) Load() (State, error) {
	var state State
	if err := config.Load(s.Path, &state); err != nil {
		return State{}, err
	}
	return state, nil
}

// Save writes the state.
func (s Store) Save(state State) error {
	return config.Save(s.Path, state)
}
