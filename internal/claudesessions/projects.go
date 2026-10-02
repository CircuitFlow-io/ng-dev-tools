package claudesessions

import (
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// folderNameUnsafe is what Claude Code replaces with a dash when it names the folder holding a
// directory's sessions: /Users/me/projects/app keeps them in -Users-me-projects-app.
var folderNameUnsafe = regexp.MustCompile(`[^a-zA-Z0-9]`)

// SessionsFolder is the folder under dir holding the sessions started in path.
func SessionsFolder(dir, path string) string {
	return filepath.Join(dir, folderNameUnsafe.ReplaceAllString(path, "-"))
}

// LastActiveIn maps each of paths to when a session started in it was last written to, leaving
// out the paths that have none. Only a transcript's modification time is read, so it is cheap
// enough to order a project list by.
func LastActiveIn(dir string, paths []string) map[string]time.Time {
	active := map[string]time.Time{}
	for _, path := range paths {
		if at := newestTranscript(SessionsFolder(dir, path)); !at.IsZero() {
			active[path] = at
		}
	}
	return active
}

func newestTranscript(folder string) time.Time {
	entries, err := os.ReadDir(folder)
	if err != nil {
		return time.Time{}
	}
	var newest time.Time
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != transcriptExt {
			continue
		}
		if info, err := entry.Info(); err == nil && info.ModTime().After(newest) {
			newest = info.ModTime()
		}
	}
	return newest
}
