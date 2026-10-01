package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJiraHostAcceptsHostsAndAddresses(t *testing.T) {
	tests := []struct {
		value, want string
	}{
		{"acme.atlassian.net", "acme.atlassian.net"},
		{" https://acme.atlassian.net/ ", "acme.atlassian.net"},
		{"https://acme.atlassian.net/browse/TS-234455", "acme.atlassian.net"},
		{"jira.corp.example/jira", "jira.corp.example/jira"},
		{"http://jira.local:8080", "http://jira.local:8080"},
	}
	key := mustLookup(t, "jira-host")
	for _, tt := range tests {
		var s Settings
		if err := key.Set(&s, tt.value, Env{}); err != nil {
			t.Errorf("set %q: %v", tt.value, err)
			continue
		}
		if s.JiraHost != tt.want {
			t.Errorf("set %q stored %q, want %q", tt.value, s.JiraHost, tt.want)
		}
	}
}

func TestJiraHostRejectsNonWebAddresses(t *testing.T) {
	key := mustLookup(t, "jira-host")
	for _, value := range []string{"", "ftp://acme.example", "https://"} {
		s := Settings{JiraHost: "kept.example"}
		if err := key.Set(&s, value, Env{}); err == nil {
			t.Errorf("set %q: no error", value)
		}
		if s.JiraHost != "kept.example" {
			t.Errorf("set %q changed the setting to %q", value, s.JiraHost)
		}
	}
}

func TestTicketURLPrefix(t *testing.T) {
	tests := map[string]string{
		"":                       "",
		"acme.atlassian.net":     "https://acme.atlassian.net/browse/",
		"http://jira.local:8080": "http://jira.local:8080/browse/",
	}
	for host, want := range tests {
		if got := (Settings{JiraHost: host}).TicketURLPrefix(); got != want {
			t.Errorf("TicketURLPrefix for %q = %q, want %q", host, got, want)
		}
	}
}

func TestProjectsDirResolvesToAnExistingFolder(t *testing.T) {
	home := t.TempDir()
	work := filepath.Join(home, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	key := mustLookup(t, "projects-dir")
	env := Env{Home: home, Dir: home}
	for _, value := range []string{"~/work", "work", work + "/", "./work/../work"} {
		var s Settings
		if err := key.Set(&s, value, env); err != nil {
			t.Errorf("set %q: %v", value, err)
			continue
		}
		if s.ProjectsDir != work {
			t.Errorf("set %q stored %q, want %q", value, s.ProjectsDir, work)
		}
	}
}

func TestProjectsDirRejectsMissingFoldersAndFiles(t *testing.T) {
	home := t.TempDir()
	file := filepath.Join(home, "notes.txt")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	key := mustLookup(t, "projects-dir")
	for _, value := range []string{"", "~/missing", file} {
		var s Settings
		if err := key.Set(&s, value, Env{Home: home, Dir: home}); err == nil {
			t.Errorf("set %q: no error", value)
		}
	}
}

func TestProjectsRootDefaultsToHomeProjects(t *testing.T) {
	if got := (Settings{}).ProjectsRoot("/Users/me"); got != "/Users/me/projects" {
		t.Errorf("default projects root = %q", got)
	}
	if got := (Settings{ProjectsDir: "/Users/me/work"}).ProjectsRoot("/Users/me"); got != "/Users/me/work" {
		t.Errorf("set projects root = %q", got)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "ngt", "settings.json")}
	empty, err := store.Load()
	if err != nil || empty != (Settings{}) {
		t.Fatalf("load before save = %+v, %v", empty, err)
	}
	want := Settings{ProjectsDir: "/Users/me/work", JiraHost: "acme.atlassian.net"}
	if err := store.Save(want); err != nil {
		t.Fatal(err)
	}
	if got, err := store.Load(); err != nil || got != want {
		t.Errorf("load after save = %+v, %v; want %+v", got, err, want)
	}
}

func TestLookupNamesTheKnownSettings(t *testing.T) {
	_, err := Lookup("jira")
	if err == nil || err.Error() != `unknown setting "jira" (known: projects-dir, jira-host)` {
		t.Errorf("Lookup error = %v", err)
	}
}

func mustLookup(t *testing.T, name string) Key {
	t.Helper()
	key, err := Lookup(name)
	if err != nil {
		t.Fatal(err)
	}
	return key
}
