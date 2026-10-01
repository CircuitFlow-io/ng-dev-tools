package cli

import (
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

type projectsJSON struct {
	Projects []projectJSON `json:"projects"`
}

type projectJSON struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	Git    bool   `json:"git"`
	Branch string `json:"branch,omitempty"`
	// Opened is when ngt last opened the project; Changed is its newest file, commit or checkout.
	Opened       time.Time `json:"opened,omitzero"`
	Changed      time.Time `json:"changed,omitzero"`
	LastActivity time.Time `json:"lastActivity,omitzero"`
}

func toProjectsJSON(found []projects.Project) projectsJSON {
	views := make([]projectJSON, 0, len(found))
	for _, p := range found {
		views = append(views, projectJSON{
			Name:         p.Name,
			Path:         p.Path,
			Git:          p.Git,
			Branch:       p.Branch,
			Opened:       p.Opened,
			Changed:      p.Changed,
			LastActivity: p.LastActivity(),
		})
	}
	return projectsJSON{Projects: views}
}
