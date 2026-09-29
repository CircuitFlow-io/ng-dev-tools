package cleanup

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Guard is the last line of defence before a permanent delete. It only allows paths strictly
// inside a few known roots and never a well-known top-level folder itself.
type Guard struct {
	allowedRoots []string
	protected    map[string]bool
	forbidden    []string
}

// NewGuard builds a Guard for the given home directory.
func NewGuard(home string) Guard {
	inHome := func(elem ...string) string { return filepath.Join(append([]string{home}, elem...)...) }
	protected := []string{
		"/", "/Applications", "/Library", "/Library/Caches", "/Library/Logs", "/Users", "/System",
		home,
		inHome("Library"), inHome("Library", "Caches"), inHome("Library", "Logs"),
		inHome("Library", "Application Support"), inHome("Library", "Preferences"),
		inHome("Library", "Containers"), inHome("Library", "Developer"),
		inHome("Applications"), inHome("Desktop"), inHome("Documents"), inHome("Downloads"),
		inHome("Movies"), inHome("Music"), inHome("Pictures"), inHome("Public"), inHome(".Trash"),
	}
	g := Guard{
		allowedRoots: []string{home, "/Applications", "/Library/Caches", "/Library/Logs"},
		protected:    make(map[string]bool, len(protected)),
		forbidden: []string{
			inHome(".ssh"), inHome(".gnupg"), inHome("Library", "Keychains"),
			inHome("Library", "Mobile Documents"), inHome("Library", "Mail"),
		},
	}
	for _, p := range protected {
		g.protected[p] = true
	}
	return g
}

// Check returns an error if path must not be deleted.
func (g Guard) Check(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return fmt.Errorf("refusing to delete non-canonical path %q", path)
	}
	if g.protected[path] {
		return fmt.Errorf("refusing to delete protected folder %q", path)
	}
	for _, root := range g.forbidden {
		if isWithin(path, root) || path == root {
			return fmt.Errorf("refusing to delete %q inside %q", path, root)
		}
	}
	for _, root := range g.allowedRoots {
		if isWithin(path, root) {
			return nil
		}
	}
	return fmt.Errorf("refusing to delete %q outside allowed locations", path)
}

// isWithin reports whether path is strictly below dir.
func isWithin(path, dir string) bool {
	return strings.HasPrefix(path, strings.TrimSuffix(dir, "/")+"/")
}
