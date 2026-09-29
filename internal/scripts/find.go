package scripts

import "path/filepath"

// FindRoot looks upwards from cwd for the project it belongs to, stopping at home and at the
// top of the git repository. root is the monorepo root when cwd is inside one, otherwise the
// nearest folder with a package.json.
func FindRoot(cwd, home string) (root string, ok bool) {
	nearest := ""
	for dir := cwd; dir != home; dir = filepath.Dir(dir) {
		m, err := readManifest(dir)
		hasManifest := err == nil
		if hasManifest && nearest == "" {
			nearest = dir
		}
		if exists(filepath.Join(dir, pnpmWorkspaceFile)) || (hasManifest && hasWorkspaces(m)) {
			return dir, true
		}
		if exists(filepath.Join(dir, ".git")) || filepath.Dir(dir) == dir {
			break
		}
	}
	return nearest, nearest != ""
}
