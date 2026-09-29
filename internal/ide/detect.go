package ide

import (
	"cmp"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos"
)

const appExtension = ".app"

// SearchDirs are the folders applications are installed in.
func SearchDirs(home string) []string {
	return []string{"/Applications", filepath.Join(home, "Applications")}
}

// Detect finds the supported IDEs in dirs and their direct subfolders, sorted by name, newest version first.
func Detect(dirs []string) []IDE {
	var found []IDE
	for _, dir := range dirs {
		for _, app := range appBundles(dir) {
			if ide, ok := identify(app); ok {
				found = append(found, ide)
			}
		}
	}
	slices.SortFunc(found, func(a, b IDE) int {
		return cmp.Or(cmp.Compare(a.Name, b.Name), compareVersions(b.Version, a.Version), cmp.Compare(a.AppPath, b.AppPath))
	})
	return found
}

func appBundles(dir string) []string {
	var apps []string
	for _, name := range visibleDirs(dir) {
		path := filepath.Join(dir, name)
		if strings.HasSuffix(name, appExtension) {
			apps = append(apps, path)
			continue
		}
		for _, nested := range visibleDirs(path) {
			if strings.HasSuffix(nested, appExtension) {
				apps = append(apps, filepath.Join(path, nested))
			}
		}
	}
	return apps
}

func visibleDirs(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func identify(app string) (IDE, bool) {
	info, err := macos.ReadBundle(app)
	if err != nil {
		return IDE{}, false
	}
	p, ok := productFor(info.Identifier)
	if !ok {
		return IDE{}, false
	}
	return IDE{Name: p.name, Version: info.Version, BundleID: info.Identifier, AppPath: app}, true
}

// compareVersions orders dotted versions numerically, so "9.4" comes before "16.2".
func compareVersions(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := range min(len(as), len(bs)) {
		x, errX := strconv.Atoi(as[i])
		y, errY := strconv.Atoi(bs[i])
		if errX != nil || errY != nil {
			return cmp.Compare(a, b)
		}
		if c := cmp.Compare(x, y); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(as), len(bs))
}
