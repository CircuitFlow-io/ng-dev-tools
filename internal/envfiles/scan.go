package envfiles

import (
	"context"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
)

const (
	maxParallelScans = 8
	// maxWalkedEntries bounds the walk of a huge project; dependencies and build output are skipped.
	maxWalkedEntries = 50000
)

// generatedDirs hold dependencies and build output, whose env files and code are not the project's own.
var generatedDirs = map[string]bool{
	"node_modules": true,
	"Pods":         true,
	"build":        true,
	"dist":         true,
	"out":          true,
	"DerivedData":  true,
	"coverage":     true,
	"vendor":       true,
	"target":       true,
	"venv":         true,
	"__pycache__":  true,
}

// ScanAll reads the env files of every project in root, most urgent first.
func ScanAll(ctx context.Context, runner macos.Runner, root string) ([]Set, error) {
	dirs, err := projects.Dirs(root)
	if err != nil {
		return nil, err
	}
	found := make([][]Set, len(dirs))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelScans)
	for i, dir := range dirs {
		g.Go(func() error {
			found[i] = ScanProject(gctx, runner, root, dir)
			return nil
		})
	}
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sets := slices.Concat(found...)
	Sort(sets)
	return sets, nil
}

// ScanProject reads the env files of one project. Sets are named relative to root.
func ScanProject(ctx context.Context, runner macos.Runner, root, dir string) []Set {
	tree := walk(ctx, dir)
	p := project{root: root, known: map[string]map[string]bool{}}
	for _, folder := range slices.Sorted(maps.Keys(tree.envFiles)) {
		p.addFolder(folder, tree.envFiles[folder])
	}
	exposure, err := readGit(ctx, runner, dir)
	if err != nil {
		p.sets = append(p.sets, Set{Name: relativeName(root, dir), Dir: dir, Err: err})
	}
	p.attachExposure(exposure)
	p.attachCode(dir, tree.sources)
	return p.sets
}

// project gathers the sets of one project while it is scanned.
type project struct {
	root string
	sets []Set
	// known holds, per folder, every key its examples and local files name.
	known map[string]map[string]bool
}

func (p *project) addFolder(dir string, names []string) {
	byRole := map[Role][]string{}
	for _, name := range names {
		role, _ := classify(name)
		byRole[role] = append(byRole[role], name)
	}
	examples := byRole[Example]
	sets := make([]Set, len(examples))
	for i, example := range examples {
		sets[i] = Set{Name: relativeName(p.root, dir), Dir: dir, Example: example}
	}
	var orphans []string
	for _, local := range byRole[Local] {
		if i := closestExample(examples, local); i >= 0 {
			sets[i].Locals = append(sets[i].Locals, local)
		} else {
			orphans = append(orphans, local)
		}
	}
	for _, defaults := range byRole[Defaults] {
		if i := closestExample(examples, defaults); i >= 0 {
			sets[i].Defaults = append(sets[i].Defaults, defaults)
		}
	}
	if len(orphans) > 0 {
		sets = append(sets, Set{Name: relativeName(p.root, dir), Dir: dir, Locals: orphans})
	}
	known := map[string]bool{}
	for i := range sets {
		compare(&sets[i], known)
	}
	p.known[dir] = known
	p.sets = append(p.sets, sets...)
}

// closestExample is the index of the example with the longest base that describes the file, so
// .env.local goes with .env.local.example rather than .env.example.
func closestExample(examples []string, file string) int {
	best := -1
	for i, example := range examples {
		base := exampleBase(example)
		if describes(base, file) && (best < 0 || len(base) > len(exampleBase(examples[best]))) {
			best = i
		}
	}
	return best
}

// setFor finds the set a local file in dir belongs to, adding one without an example when there
// is none, as for a file that was deleted but is still in git history.
func (p *project) setFor(dir, file string) *Set {
	examples := map[string]int{}
	orphan := -1
	var names []string
	for i, s := range p.sets {
		if s.Dir != dir || s.Err != nil {
			continue
		}
		if s.Example == "" {
			orphan = i
			continue
		}
		examples[s.Example] = i
		names = append(names, s.Example)
	}
	if best := closestExample(names, file); best >= 0 {
		return &p.sets[examples[names[best]]]
	}
	if orphan < 0 {
		p.sets = append(p.sets, Set{Name: relativeName(p.root, dir), Dir: dir})
		orphan = len(p.sets) - 1
	}
	return &p.sets[orphan]
}

type tree struct {
	// envFiles are the env file names in each folder.
	envFiles map[string][]string
	sources  []string
}

func walk(ctx context.Context, dir string) tree {
	t := tree{envFiles: map[string][]string{}}
	walked := 0
	_ = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		walked++
		switch {
		case ctx.Err() != nil || walked > maxWalkedEntries:
			return filepath.SkipAll
		case err != nil:
			return skipUnreadable(entry)
		case entry.IsDir():
			return skipGenerated(dir, path, entry.Name())
		}
		name := entry.Name()
		if _, ok := classify(name); ok {
			folder := filepath.Dir(path)
			t.envFiles[folder] = append(t.envFiles[folder], name)
		} else if isSource(name) {
			t.sources = append(t.sources, path)
		}
		return nil
	})
	return t
}

func skipGenerated(root, path, name string) error {
	if path != root && (strings.HasPrefix(name, ".") || generatedDirs[name]) {
		return filepath.SkipDir
	}
	return nil
}

func skipUnreadable(entry fs.DirEntry) error {
	if entry != nil && entry.IsDir() {
		return filepath.SkipDir
	}
	return nil
}

func relativeName(root, dir string) string {
	name, err := filepath.Rel(root, dir)
	if err != nil {
		return filepath.Base(dir)
	}
	return name
}
