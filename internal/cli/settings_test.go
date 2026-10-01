package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/settings"
)

// isolateSettings points home and the config folder at a temporary folder and returns home.
func isolateSettings(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	return home
}

func TestSettingsSetThenListAndGet(t *testing.T) {
	home := isolateSettings(t)
	work := filepath.Join(home, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runSettingsSet(&out, outputText, "projects-dir", "~/work"); err != nil {
		t.Fatal(err)
	}
	if err := runSettingsSet(&out, outputText, "jira-host", "https://acme.atlassian.net/"); err != nil {
		t.Fatal(err)
	}

	out.Reset()
	if err := runSettingsList(&out, outputText); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"projects-dir  ~/work ", "jira-host     acme.atlassian.net "} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list misses %q:\n%s", want, out.String())
		}
	}

	out.Reset()
	if err := runSettingsGet(&out, outputText, "projects-dir"); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != work {
		t.Errorf("get projects-dir = %q, want %q", got, work)
	}
}

func TestSettingsListMarksDefaults(t *testing.T) {
	isolateSettings(t)
	var out bytes.Buffer
	if err := runSettingsList(&out, outputText); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"~/projects (default)", "jira-host     " + notSet} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("list misses %q:\n%s", want, out.String())
		}
	}
}

func TestSettingsUnsetRestoresTheDefault(t *testing.T) {
	isolateSettings(t)
	var out bytes.Buffer
	if err := runSettingsSet(&out, outputText, "jira-host", "acme.atlassian.net"); err != nil {
		t.Fatal(err)
	}
	if err := runSettingsUnset(&out, outputJSON, "jira-host"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"isSet": false`) {
		t.Errorf("unset JSON:\n%s", out.String())
	}
}

func TestSettingsRefuseToReplaceAnUnreadableFile(t *testing.T) {
	home := isolateSettings(t)
	path := filepath.Join(home, ".config", "ngt", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runSettingsSet(&bytes.Buffer{}, outputText, "jira-host", "acme.atlassian.net"); err == nil {
		t.Fatal("set over a broken file: no error")
	}
	if data, _ := os.ReadFile(path); string(data) != "{broken" {
		t.Errorf("broken file was replaced with %q", data)
	}
}

func TestProjectsRootPrefersTheFlagThenTheSetting(t *testing.T) {
	ctx := context.WithValue(context.Background(), settingsKey{}, settings.Settings{ProjectsDir: "/Users/me/work"})
	if got := projectsRoot(ctx, "/tmp/given", "/Users/me"); got != "/tmp/given" {
		t.Errorf("with --root: %q", got)
	}
	if got := projectsRoot(ctx, "", "/Users/me"); got != "/Users/me/work" {
		t.Errorf("with the setting: %q", got)
	}
	if got := projectsRoot(context.Background(), "", "/Users/me"); got != "/Users/me/projects" {
		t.Errorf("default: %q", got)
	}
}

func TestCleanSearchesTheProjectsDirOnce(t *testing.T) {
	got := projectDirs("/Users/me", "/Users/me/Developer", nil)
	want := []string{"/Users/me/Developer", "/Users/me/code", "/Users/me/src", "/Users/me/workspace"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("projectDirs = %v, want %v", got, want)
	}
}
