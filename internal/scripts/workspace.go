// Package scripts finds the package.json scripts of an npm or pnpm project, including every
// package of a monorepo, and runs them.
package scripts

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	manifestFile = "package.json"
	preHook      = "pre"
	postHook     = "post"
)

// Script is one entry of a package.json "scripts" object. Pre and Post are the commands of its
// pre and post hooks, which are not listed as scripts of their own.
type Script struct {
	Name    string
	Command string
	Pre     string
	Post    string
}

// Package is a folder with a package.json and at least one script.
type Package struct {
	Name string
	Dir  string
	// RelDir is Dir relative to the workspace root, "" for the root itself.
	RelDir  string
	Scripts []Script
}

// IsRoot reports whether this is the workspace's root package.
func (p Package) IsRoot() bool {
	return p.RelDir == ""
}

// Script finds the script called name.
func (p Package) Script(name string) (Script, bool) {
	i := slices.IndexFunc(p.Scripts, func(s Script) bool { return s.Name == name })
	if i < 0 {
		return Script{}, false
	}
	return p.Scripts[i], true
}

// Workspace is a project and, for a monorepo, its member packages.
type Workspace struct {
	Root    string
	Manager Manager
	// RunsHooks reports whether the package manager runs pre and post hooks.
	RunsHooks bool
	// Packages holds the root first, then the members by folder. Packages without scripts are left out.
	Packages []Package
}

// Name is the project's folder name.
func (w Workspace) Name() string {
	return filepath.Base(w.Root)
}

// ScriptCount is the number of scripts over all packages.
func (w Workspace) ScriptCount() int {
	count := 0
	for _, p := range w.Packages {
		count += len(p.Scripts)
	}
	return count
}

// Package finds the package in relDir.
func (w Workspace) Package(relDir string) (Package, bool) {
	i := slices.IndexFunc(w.Packages, func(p Package) bool { return p.RelDir == relDir })
	if i < 0 {
		return Package{}, false
	}
	return w.Packages[i], true
}

// PackageFor is the package whose folder holds dir, the deepest one when packages are nested.
func (w Workspace) PackageFor(dir string) (Package, bool) {
	var found Package
	ok := false
	for _, p := range w.Packages {
		if isWithin(dir, p.Dir) && (!ok || len(p.Dir) > len(found.Dir)) {
			found, ok = p, true
		}
	}
	return found, ok
}

// Target is one script of one package.
type Target struct {
	Package Package
	Script  Script
}

// Targets lists every script, in package order.
func (w Workspace) Targets() []Target {
	var targets []Target
	for _, p := range w.Packages {
		for _, s := range p.Scripts {
			targets = append(targets, Target{Package: p, Script: s})
		}
	}
	return targets
}

// Find looks up the script called script in the package in relDir.
func (w Workspace) Find(relDir, script string) (Target, bool) {
	p, ok := w.Package(relDir)
	if !ok {
		return Target{}, false
	}
	s, ok := p.Script(script)
	return Target{Package: p, Script: s}, ok
}

type manifest struct {
	Name           string          `json:"name"`
	PackageManager string          `json:"packageManager"`
	Scripts        json.RawMessage `json:"scripts"`
	Workspaces     json.RawMessage `json:"workspaces"`
}

// Load reads the project at root: its package manager, its root package and its members.
func Load(root string) (Workspace, error) {
	rootManifest, err := readManifest(root)
	if err != nil {
		return Workspace{}, err
	}
	manager, err := detectManager(root, rootManifest)
	if err != nil {
		return Workspace{}, err
	}
	settings, err := readPNPMWorkspace(root)
	if err != nil {
		return Workspace{}, err
	}
	ws := Workspace{Root: root, Manager: manager, RunsHooks: runsHooks(root, manager, settings)}

	patterns, err := memberPatterns(settings, rootManifest)
	if err != nil {
		return Workspace{}, err
	}
	if err := ws.add(root, "", rootManifest); err != nil {
		return Workspace{}, err
	}
	members, err := findMembers(root, patterns)
	if err != nil {
		return Workspace{}, err
	}
	for _, rel := range members {
		dir := filepath.Join(root, rel)
		m, err := readManifest(dir)
		if err != nil {
			return Workspace{}, err
		}
		if err := ws.add(dir, rel, m); err != nil {
			return Workspace{}, err
		}
	}
	slices.SortStableFunc(ws.Packages[min(1, len(ws.Packages)):], func(a, b Package) int {
		return cmp.Compare(a.RelDir, b.RelDir)
	})
	return ws, nil
}

func (w *Workspace) add(dir, rel string, m manifest) error {
	scripts, err := orderedScripts(m.Scripts)
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Join(dir, manifestFile), err)
	}
	scripts = attachHooks(scripts)
	if len(scripts) == 0 {
		return nil
	}
	w.Packages = append(w.Packages, Package{Name: m.Name, Dir: dir, RelDir: rel, Scripts: scripts})
	return nil
}

func readManifest(dir string) (manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		return manifest{}, err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return manifest{}, fmt.Errorf("%s: %w", filepath.Join(dir, manifestFile), err)
	}
	return m, nil
}

var errScriptsNotObject = errors.New(`"scripts" is not an object`)

// orderedScripts decodes the "scripts" object keeping the order it is written in, which a Go map
// would lose.
func orderedScripts(raw json.RawMessage) ([]Script, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, errScriptsNotObject
	}
	var scripts []Script
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		var command string
		if err := dec.Decode(&command); err != nil {
			return nil, err
		}
		scripts = append(scripts, Script{Name: tok.(string), Command: strings.TrimSpace(command)})
	}
	return scripts, nil
}

// attachHooks moves each "preX" and "postX" script onto X, when X exists.
func attachHooks(scripts []Script) []Script {
	commands := map[string]string{}
	for _, s := range scripts {
		commands[s.Name] = s.Command
	}
	isHook := func(name string) bool {
		for _, prefix := range []string{preHook, postHook} {
			if target, ok := strings.CutPrefix(name, prefix); ok && target != "" {
				if _, exists := commands[target]; exists {
					return true
				}
			}
		}
		return false
	}

	kept := make([]Script, 0, len(scripts))
	for _, s := range scripts {
		if isHook(s.Name) {
			continue
		}
		s.Pre, s.Post = commands[preHook+s.Name], commands[postHook+s.Name]
		kept = append(kept, s)
	}
	return kept
}

func isWithin(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
