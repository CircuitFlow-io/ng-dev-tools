// Package settings holds the preferences set with ngt settings, kept in the user's config folder.
package settings

import (
	"github.com/CircuitFlow-io/ng-dev-tools/internal/config"
)

const (
	fileName = "settings.json"
	// DefaultProjectsDir is the projects folder, relative to home, when none is set.
	DefaultProjectsDir = "projects"
	defaultJiraScheme  = "https://"
	jiraTicketPath     = "/browse/"
)

// Settings are the user's preferences. Empty fields mean the default. The JSON keys are the names
// ngt settings uses, so the file can be edited by hand.
type Settings struct {
	// ProjectsDir is the absolute path of the folder that holds the user's projects.
	ProjectsDir string `json:"projectsDir,omitempty"`
	// JiraHost is the Jira site ticket keys link to, such as "acme.atlassian.net". It keeps its
	// scheme only when that is not https.
	JiraHost string `json:"jiraHost,omitempty"`
}

// ProjectsRoot is the folder that holds the user's projects: ProjectsDir, or ~/projects.
func (s Settings) ProjectsRoot(home string) string {
	if s.ProjectsDir != "" {
		return s.ProjectsDir
	}
	return defaultProjectsRoot(home)
}

// TicketURLPrefix is what a ticket key is appended to for its Jira page, or "" without a Jira host.
func (s Settings) TicketURLPrefix() string {
	if s.JiraHost == "" {
		return ""
	}
	return withScheme(s.JiraHost) + jiraTicketPath
}

// Store keeps Settings in a JSON file.
type Store struct {
	Path string
}

// DefaultStore is $XDG_CONFIG_HOME/ngt/settings.json, or ~/.config/ngt/settings.json.
func DefaultStore(home string, getenv func(string) string) Store {
	return Store{Path: config.Path(home, getenv, fileName)}
}

// Load reads the settings as written, which are all defaults before the first save. An unknown key
// is an error. Validated checks and normalizes the values.
func (s Store) Load() (Settings, error) {
	var loaded Settings
	if err := config.LoadStrict(s.Path, &loaded); err != nil {
		return Settings{}, err
	}
	return loaded, nil
}

// Save writes the settings.
func (s Store) Save(settings Settings) error {
	return config.Save(s.Path, settings)
}
