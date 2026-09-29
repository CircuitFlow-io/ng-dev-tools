package doctor

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nasserghiasi/ng-dev-tools/internal/macos/macostest"
)

var errNoRelease = errors.New("no release")

// machine is a fake Mac: scripted commands, environment variables, tools on PATH and a temp home.
type machine struct {
	t        *testing.T
	runner   *macostest.Runner
	vars     map[string]string
	tools    map[string]string
	probe    *fakeProbe
	releases *fakeReleases
	home     string
	root     string
}

func newMachine(t *testing.T) *machine {
	t.Helper()
	return &machine{
		t:        t,
		runner:   &macostest.Runner{Outputs: map[string]string{}, Errors: map[string]error{}},
		vars:     map[string]string{},
		tools:    map[string]string{},
		probe:    &fakeProbe{},
		releases: &fakeReleases{npm: map[string]Version{}},
		home:     t.TempDir(),
		root:     t.TempDir(),
	}
}

func (m *machine) env() Env {
	return Env{
		Runner:   m.runner,
		Probe:    m.probe,
		Releases: m.releases,
		Home:     m.home,
		Root:     m.root,
		Getenv:   func(key string) string { return m.vars[key] },
		LookPath: func(name string) (string, error) {
			if path, ok := m.tools[name]; ok {
				return path, nil
			}
			return "", exec.ErrNotFound
		},
		Now: time.Date(2026, 9, 29, 12, 0, 0, 0, time.Local),
	}
}

// install puts a tool on PATH whose command prints output.
func (m *machine) install(name, command, output string) {
	m.tools[name] = "/opt/homebrew/bin/" + name
	if command != "" {
		m.runner.Outputs[command] = output
	}
}

func (m *machine) failing(command string, err error) {
	m.runner.Errors[command] = err
}

// file creates path (relative to home) and its parent directories.
func (m *machine) file(path string) {
	m.t.Helper()
	full := filepath.Join(m.home, path)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		m.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte("x"), 0o644); err != nil {
		m.t.Fatal(err)
	}
}

func (m *machine) dir(path string) string {
	m.t.Helper()
	full := filepath.Join(m.home, path)
	if err := os.MkdirAll(full, 0o755); err != nil {
		m.t.Fatal(err)
	}
	return full
}

func (m *machine) run(check func(context.Context, Env) Result) Result {
	return check(context.Background(), m.env())
}

type fakeProbe struct {
	free      uint64
	openFiles uint64
	latency   time.Duration
	reachErr  error
	dnsErr    error
}

func (p *fakeProbe) DiskFree(string) (uint64, error)          { return p.free, nil }
func (p *fakeProbe) OpenFilesLimit() (uint64, error)          { return p.openFiles, nil }
func (p *fakeProbe) LookupHost(context.Context, string) error { return p.dnsErr }

func (p *fakeProbe) Reach(context.Context, string) (time.Duration, error) {
	return p.latency, p.reachErr
}

type fakeReleases struct {
	node, golang Version
	npm          map[string]Version
}

func (r *fakeReleases) NodeLTS(context.Context) (Version, error) { return r.node, nil }
func (r *fakeReleases) Go(context.Context) (Version, error)      { return r.golang, nil }

func (r *fakeReleases) Npm(_ context.Context, pkg string) (Version, error) {
	v, ok := r.npm[pkg]
	if !ok {
		return Version{}, errNoRelease
	}
	return v, nil
}

func assertStatus(t *testing.T, got Result, want Status) {
	t.Helper()
	if got.Status != want {
		t.Errorf("status = %v (%q, details %q), want %v", got.Status, got.Summary, got.Details, want)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("%q does not contain %q", got, want)
	}
}
