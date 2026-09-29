package doctor

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const (
	httpTimeout = 5 * time.Second
	// maxErrorLength keeps a command's error readable; the full text is one rerun away.
	maxErrorLength = 120
)

var errNotInstalled = errors.New("not installed")

// Env is everything checks read from the machine. Tests replace each part with a fake.
type Env struct {
	Runner   macos.Runner
	Probe    Probe
	Releases Releases
	Home     string
	// Root prefixes absolute system paths, so tests can lay out a fake filesystem.
	Root     string
	Getenv   func(key string) string
	LookPath func(name string) (string, error)
	Now      time.Time
}

// NewEnv reads the real machine.
func NewEnv(home string) Env {
	client := &http.Client{Timeout: httpTimeout}
	return Env{
		Runner:   macos.ExecRunner{},
		Probe:    SystemProbe{Client: client},
		Releases: WebReleases{Client: client},
		Home:     home,
		Getenv:   os.Getenv,
		LookPath: exec.LookPath,
		Now:      time.Now(),
	}
}

func (e Env) homePath(parts ...string) string {
	return filepath.Join(append([]string{e.Home}, parts...)...)
}

func (e Env) systemPath(path string) string {
	return filepath.Join(e.Root, path)
}

func (e Env) installed(name string) bool {
	_, err := e.LookPath(name)
	return err == nil
}

// output runs a command and returns its trimmed stdout.
func (e Env) output(ctx context.Context, name string, args ...string) (string, error) {
	out, err := e.Runner.Run(ctx, name, args...)
	return strings.TrimSpace(string(out)), err
}

// version runs a tool's version command and parses the release number it prints.
func (e Env) version(ctx context.Context, name string, args ...string) (Version, error) {
	if !e.installed(name) {
		return Version{}, errNotInstalled
	}
	out, err := e.output(ctx, name, args...)
	if err != nil {
		return Version{}, err
	}
	v, ok := ParseVersion(out)
	if !ok {
		return Version{}, fmt.Errorf("unrecognised version output %q", firstLine(out))
	}
	return v, nil
}

// pathEntries lists the directories on PATH in order.
func (e Env) pathEntries() []string {
	return slices.DeleteFunc(strings.Split(e.Getenv("PATH"), ":"), func(dir string) bool { return dir == "" })
}

func (e Env) onPath(dir string) bool {
	return slices.Contains(e.pathEntries(), filepath.Clean(dir))
}

// subdirs lists the names of the directories inside dir.
func subdirs(dir string) []string {
	entries, _ := os.ReadDir(dir)
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return line
}

// errorLine picks the most telling line of a failed command's error: the first one starting
// with "Error", or the last non-empty one.
func errorLine(err error) string {
	lines := slices.DeleteFunc(strings.Split(err.Error(), "\n"), func(line string) bool {
		return strings.TrimSpace(line) == ""
	})
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Error") {
			return ui.Truncate(strings.TrimSpace(line), maxErrorLength)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return ui.Truncate(strings.TrimSpace(lines[len(lines)-1]), maxErrorLength)
}

// missing explains why a tool's version could not be read.
func missing(err error, fix string) Result {
	if errors.Is(err, errNotInstalled) {
		return fail("not installed", fix)
	}
	return fail("could not read its version", fix).with(errorLine(err))
}

// compareWithLatest passes when current is the newest release, and warns when an update exists
// or the newest release could not be looked up.
func compareWithLatest(current, latest Version, err error, fix string) Result {
	switch {
	case errors.Is(err, ErrOffline):
		return pass(current.String() + ", latest not checked (offline)")
	case err != nil:
		return warn(current.String()+", could not look up the latest release", "").with(err.Error())
	case current.Less(latest):
		return warn(fmt.Sprintf("%s, %s is available", current, latest), fix)
	default:
		return pass(current.String() + ", up to date")
	}
}
