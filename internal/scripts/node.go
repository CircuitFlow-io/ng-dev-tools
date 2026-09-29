package scripts

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

var nodeVersionFiles = []string{".nvmrc", ".node-version"}

// Node is the Node.js version a package asks for, and where nvm has it installed.
type Node struct {
	// Spec is the version as written, such as "24" or "v22.13.1", "" when none is asked for.
	Spec string
	// Source is the file Spec comes from.
	Source string
	// Version is the installed version Spec resolves to, such as "24.13.0".
	Version string
	// Bin is the folder holding that version's node, "" when it is not installed.
	Bin string
}

// Installed reports whether the version asked for is installed.
func (n Node) Installed() bool {
	return n.Bin != ""
}

// NVMDir is $NVM_DIR, or ~/.nvm.
func NVMDir(home string, getenv func(string) string) string {
	return cmp.Or(getenv("NVM_DIR"), filepath.Join(home, ".nvm"))
}

// RequiredNode reads the version asked for by the nearest .nvmrc or .node-version between dir and
// root, and finds it among nvm's installed versions.
func RequiredNode(dir, root, nvmDir string) Node {
	spec, source := nodeSpec(dir, root)
	if spec == "" {
		return Node{}
	}
	node := Node{Spec: spec, Source: source}
	node.Version, node.Bin = resolveNVM(nvmDir, spec)
	return node
}

func nodeSpec(dir, root string) (spec, source string) {
	for {
		for _, name := range nodeVersionFiles {
			data, err := os.ReadFile(filepath.Join(dir, name))
			if err == nil && strings.TrimSpace(string(data)) != "" {
				return strings.TrimSpace(string(data)), name
			}
		}
		if dir == root || filepath.Dir(dir) == dir {
			return "", ""
		}
		dir = filepath.Dir(dir)
	}
}

// resolveNVM finds the highest installed version starting with spec's numbers: "24" picks the
// newest 24.x, "22.13.1" only that one. Aliases such as "lts/*" are not resolved.
func resolveNVM(nvmDir, spec string) (version, bin string) {
	want, ok := parseVersion(spec)
	if !ok {
		return "", ""
	}
	versionsDir := filepath.Join(nvmDir, "versions", "node")
	entries, err := os.ReadDir(versionsDir)
	if err != nil {
		return "", ""
	}
	var best []int
	for _, e := range entries {
		have, ok := parseVersion(e.Name())
		if !ok || len(have) < len(want) || !slices.Equal(have[:len(want)], want) {
			continue
		}
		if best == nil || slices.Compare(have, best) > 0 {
			best, version = have, strings.TrimPrefix(e.Name(), "v")
		}
	}
	if best == nil {
		return "", ""
	}
	return version, filepath.Join(versionsDir, "v"+version, "bin")
}

func parseVersion(s string) ([]int, bool) {
	fields := strings.Split(strings.TrimPrefix(strings.TrimSpace(s), "v"), ".")
	numbers := make([]int, len(fields))
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			return nil, false
		}
		numbers[i] = n
	}
	return numbers, true
}

// MissingModules reports whether dependencies are not installed yet.
func (w Workspace) MissingModules() bool {
	return !exists(filepath.Join(w.Root, nodeModules))
}
