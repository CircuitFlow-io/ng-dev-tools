// Package config reads and writes ngt's JSON state files in the user's config folder.
package config

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

const dirPermissions = 0o755

// Path is $XDG_CONFIG_HOME/ngt/file, or ~/.config/ngt/file.
func Path(home string, getenv func(string) string, file string) string {
	base := getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "ngt", file)
}

// Load decodes the JSON file at path into v, leaving v untouched when the file does not exist yet.
func Load(path string, v any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// Save writes v as JSON through a temporary file, so an interrupted save never leaves a broken file.
func Save(path string, v any) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return err
	}
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
