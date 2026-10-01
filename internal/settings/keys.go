package settings

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	homePrefix  = "~"
	schemeSep   = "://"
	httpScheme  = "http"
	httpsScheme = "https"
)

// Env is what normalizing a value depends on.
type Env struct {
	Home string
	// Dir is the working directory, which relative paths are resolved against.
	Dir string
}

// Key is one setting as ngt settings names it.
type Key struct {
	Name        string
	Description string
	// Default is the value used when the setting is not set, "" when there is none.
	Default string
	value   func(Settings, string) string
	isSet   func(Settings) bool
	set     func(*Settings, string, Env) error
	unset   func(*Settings)
}

// Keys are every setting, in the order they are listed.
var Keys = []Key{
	{
		Name:        "projects-dir",
		Description: "folder that holds your projects, used by every command with a --root flag",
		Default:     homePrefix + "/" + DefaultProjectsDir,
		value:       func(s Settings, home string) string { return s.ProjectsRoot(home) },
		isSet:       func(s Settings) bool { return s.ProjectsDir != "" },
		set: func(s *Settings, value string, env Env) error {
			dir, err := normalizeProjectsDir(value, env)
			s.ProjectsDir = dir
			return err
		},
		unset: func(s *Settings) { s.ProjectsDir = "" },
	},
	{
		Name:        "jira-host",
		Description: "Jira site that ticket keys such as TS-1234 link to, e.g. acme.atlassian.net",
		value:       func(s Settings, _ string) string { return s.JiraHost },
		isSet:       func(s Settings) bool { return s.JiraHost != "" },
		set: func(s *Settings, value string, _ Env) error {
			host, err := normalizeJiraHost(value)
			s.JiraHost = host
			return err
		},
		unset: func(s *Settings) { s.JiraHost = "" },
	},
}

// Lookup finds the setting called name.
func Lookup(name string) (Key, error) {
	names := make([]string, 0, len(Keys))
	for _, k := range Keys {
		if k.Name == name {
			return k, nil
		}
		names = append(names, k.Name)
	}
	return Key{}, fmt.Errorf("unknown setting %q (known: %s)", name, strings.Join(names, ", "))
}

// Value is the setting's effective value, the default when it is not set. It is "" for a setting
// without a default value.
func (k Key) Value(s Settings, home string) string {
	return k.value(s, home)
}

// IsSet reports whether the setting has a value of its own rather than the default.
func (k Key) IsSet(s Settings) bool {
	return k.isSet(s)
}

// Set validates value and stores it, normalized, in s. On error s is left unchanged.
func (k Key) Set(s *Settings, value string, env Env) error {
	updated := *s
	if err := k.set(&updated, strings.TrimSpace(value), env); err != nil {
		return fmt.Errorf("%s: %w", k.Name, err)
	}
	*s = updated
	return nil
}

// Unset puts the setting back to its default.
func (k Key) Unset(s *Settings) {
	k.unset(s)
}

func defaultProjectsRoot(home string) string {
	return filepath.Join(home, DefaultProjectsDir)
}

// normalizeProjectsDir turns value into the absolute path of an existing folder, expanding a
// leading ~ the shell left alone because it was quoted.
func normalizeProjectsDir(value string, env Env) (string, error) {
	if value == "" {
		return "", errors.New("the folder is empty")
	}
	dir := expandHome(value, env.Home)
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(env.Dir, dir)
	}
	dir = filepath.Clean(dir)
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return "", fmt.Errorf("%s does not exist", dir)
	}
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a folder", dir)
	}
	return dir, nil
}

func expandHome(path, home string) string {
	if path == homePrefix {
		return home
	}
	if rest, ok := strings.CutPrefix(path, homePrefix+"/"); ok {
		return filepath.Join(home, rest)
	}
	return path
}

// normalizeJiraHost accepts a host such as "acme.atlassian.net" or any URL on the site, such as a
// ticket's page, and keeps its host and any folder Jira is served from.
func normalizeJiraHost(value string) (string, error) {
	if value == "" {
		return "", errors.New("the host is empty")
	}
	if !strings.Contains(value, schemeSep) {
		value = defaultJiraScheme + value
	}
	u, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if u.Scheme != httpsScheme && u.Scheme != httpScheme {
		return "", fmt.Errorf("%s is not a web address", value)
	}
	if u.Host == "" {
		return "", fmt.Errorf("%s has no host name", value)
	}
	path, _, _ := strings.Cut(u.Path, jiraTicketPath)
	host := u.Host + strings.TrimRight(path, "/")
	if u.Scheme == httpScheme {
		return httpScheme + schemeSep + host, nil
	}
	return host, nil
}

func withScheme(host string) string {
	if strings.Contains(host, schemeSep) {
		return host
	}
	return defaultJiraScheme + host
}
