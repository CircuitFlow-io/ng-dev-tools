package scripts

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Manager is the package manager that runs a project's scripts.
type Manager string

const (
	NPM  Manager = "npm"
	PNPM Manager = "pnpm"
)

const (
	pnpmLockFile        = "pnpm-lock.yaml"
	npmLockFile         = "package-lock.json"
	npmrcFile           = ".npmrc"
	npmrcPrePostSetting = "enable-pre-post-scripts"
)

var unsupportedLockFiles = map[string]string{
	"yarn.lock": "yarn",
	"bun.lock":  "bun",
	"bun.lockb": "bun",
}

// UnsupportedError reports a project that uses a package manager other than npm or pnpm.
type UnsupportedError struct {
	Manager string
}

func (e UnsupportedError) Error() string {
	return fmt.Sprintf("%s projects are not supported, only npm and pnpm", e.Manager)
}

// IsProject reports whether dir holds an npm or pnpm project.
func IsProject(dir string) bool {
	m, err := readManifest(dir)
	if err != nil {
		return false
	}
	_, err = detectManager(dir, m)
	return err == nil
}

// detectManager follows package.json's "packageManager" field, then the lock file, then defaults to npm.
func detectManager(root string, m manifest) (Manager, error) {
	if m.PackageManager != "" {
		name, _, _ := strings.Cut(m.PackageManager, "@")
		switch Manager(name) {
		case NPM, PNPM:
			return Manager(name), nil
		default:
			return "", UnsupportedError{Manager: name}
		}
	}
	if exists(filepath.Join(root, pnpmLockFile)) || exists(filepath.Join(root, pnpmWorkspaceFile)) {
		return PNPM, nil
	}
	if exists(filepath.Join(root, npmLockFile)) {
		return NPM, nil
	}
	for file, name := range unsupportedLockFiles {
		if exists(filepath.Join(root, file)) {
			return "", UnsupportedError{Manager: name}
		}
	}
	return NPM, nil
}

// runsHooks reports whether pre and post hooks run: always for npm, and for pnpm only when
// enablePrePostScripts is turned on.
func runsHooks(root string, manager Manager, settings pnpmWorkspace) bool {
	if manager == NPM || settings.EnablePrePostScripts {
		return true
	}
	return npmrcEnablesHooks(filepath.Join(root, npmrcFile))
}

func npmrcEnablesHooks(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && strings.TrimSpace(key) == npmrcPrePostSetting && strings.TrimSpace(value) == "true" {
			return true
		}
	}
	return false
}
