package rules

import (
	"context"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/fsx"
)

// maxProjectDepth limits how deep below a projects folder we look for build artifacts.
const maxProjectDepth = 5

// projectArtifact is a regenerable folder inside a project, recognised by a marker file next to it.
type projectArtifact struct {
	dirName string
	markers []string
	safety  cleanup.Safety
}

var (
	nodeMarkers   = []string{"package.json"}
	gradleMarkers = []string{"build.gradle", "build.gradle.kts", "settings.gradle", "settings.gradle.kts"}
	pythonMarkers = []string{"pyproject.toml", "requirements.txt", "setup.py", "Pipfile"}
)

var projectArtifacts = []projectArtifact{
	{"node_modules", nodeMarkers, cleanup.SafetySafe},
	{".next", nodeMarkers, cleanup.SafetySafe},
	{".turbo", nodeMarkers, cleanup.SafetySafe},
	{"Pods", []string{"Podfile"}, cleanup.SafetySafe},
	{"target", []string{"Cargo.toml"}, cleanup.SafetySafe},
	{".gradle", gradleMarkers, cleanup.SafetySafe},
	{"build", gradleMarkers, cleanup.SafetySafe},
	{".dart_tool", []string{"pubspec.yaml"}, cleanup.SafetySafe},
	{".venv", pythonMarkers, cleanup.SafetyReview},
	{"venv", pythonMarkers, cleanup.SafetyReview},
}

// artifactNames is used both to recognise artifacts and to ignore them when judging project activity.
var artifactNames = func() map[string]bool {
	names := make(map[string]bool, len(projectArtifacts))
	for _, a := range projectArtifacts {
		names[a.dirName] = true
	}
	return names
}()

// projectsRule finds build artifacts in projects that have not been touched recently.
type projectsRule struct{}

func (projectsRule) Name() string               { return "Stale project artifacts" }
func (projectsRule) Category() cleanup.Category { return dev }

func (projectsRule) Scan(ctx context.Context, env cleanup.Env) ([]cleanup.Item, error) {
	var items []cleanup.Item
	for _, root := range env.ProjectDirs {
		if !fsx.Exists(root) {
			continue
		}
		found, err := findStaleArtifacts(ctx, env, root)
		if err != nil {
			return items, err
		}
		items = append(items, found...)
	}
	return items, nil
}

func findStaleArtifacts(ctx context.Context, env cleanup.Env, root string) ([]cleanup.Item, error) {
	var items []cleanup.Item
	activity := make(map[string]bool)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || !d.IsDir() || path == root {
			return nil
		}
		if artifact, ok := matchArtifact(path); ok {
			if isProjectStale(env, filepath.Dir(path), activity) {
				items = append(items, artifactItem(path, artifact))
			}
			return fs.SkipDir
		}
		if strings.HasPrefix(d.Name(), ".") || artifactNames[d.Name()] || depth(root, path) >= maxProjectDepth {
			return fs.SkipDir
		}
		return nil
	})
	return items, err
}

func matchArtifact(path string) (projectArtifact, bool) {
	name, project := filepath.Base(path), filepath.Dir(path)
	for _, artifact := range projectArtifacts {
		if artifact.dirName == name && hasAnyFile(project, artifact.markers) {
			return artifact, true
		}
	}
	return projectArtifact{}, false
}

func isProjectStale(env cleanup.Env, project string, cache map[string]bool) bool {
	if stale, ok := cache[project]; ok {
		return stale
	}
	stale := env.IsStale(fsx.NewestModTime(project, artifactNames))
	cache[project] = stale
	return stale
}

func artifactItem(path string, artifact projectArtifact) cleanup.Item {
	project := filepath.Base(filepath.Dir(path))
	return cleanup.PathItem(dev, artifact.safety, artifact.dirName+" in "+project, path)
}

func hasAnyFile(dir string, names []string) bool {
	for _, name := range names {
		if fsx.Exists(filepath.Join(dir, name)) {
			return true
		}
	}
	return false
}

func depth(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return 0
	}
	return strings.Count(rel, string(filepath.Separator)) + 1
}
