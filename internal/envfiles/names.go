package envfiles

import (
	"regexp"
	"strings"
)

// Role is what an env file is for, told by its name.
type Role int

const (
	// Local files hold your own values and should never be committed: .env, .env.local, app.env.
	Local Role = iota
	// Example files list the keys and are committed: .env.example, .env.local.sample.
	Example
	// Defaults files hold shared values committed on purpose, such as fastlane's .env.default.
	Defaults
)

// envFileName matches .env, .env.<anything> and <name>.env[.<anything>], but not .xcode.env or
// .envrc, which are shell scripts.
var envFileName = regexp.MustCompile(`^([A-Za-z0-9_-]*)\.env((?:\.[A-Za-z0-9_-]+)*)$`)

var (
	exampleSuffixes  = map[string]bool{"example": true, "sample": true, "template": true, "dist": true}
	defaultsSuffixes = map[string]bool{"default": true, "defaults": true}
)

// classify tells whether name is an env file, and its role.
func classify(name string) (Role, bool) {
	m := envFileName.FindStringSubmatch(name)
	if m == nil {
		return 0, false
	}
	last := m[2][strings.LastIndex(m[2], ".")+1:]
	switch {
	case exampleSuffixes[last]:
		return Example, true
	case defaultsSuffixes[last]:
		return Defaults, true
	}
	return Local, true
}

// exampleBase is the local file an example describes: .env for .env.example, .env.local for
// .env.local.example.
func exampleBase(example string) string {
	return example[:strings.LastIndex(example, ".")]
}

// describes reports whether an example with this base covers the local file: .env.example
// covers .env, .env.local and .env.production.local.
func describes(base, local string) bool {
	return local == base || strings.HasPrefix(local, base+".")
}

// IsSecretFile reports whether name is an env file meant to hold your own values, which should
// never be committed.
func IsSecretFile(name string) bool {
	role, ok := classify(name)
	return ok && role == Local
}
