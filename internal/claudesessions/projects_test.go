package claudesessions

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSessionsFolderDashesEverythingButLettersAndDigits(t *testing.T) {
	got := SessionsFolder("/c", "/Users/me/projects/my_app.v2")
	if want := "/c/-Users-me-projects-my-app-v2"; got != want {
		t.Errorf("SessionsFolder = %q, want %q", got, want)
	}
}

func TestLastActiveInTakesTheNewestTranscriptOfEachProject(t *testing.T) {
	dir := t.TempDir()
	older, newer := time.Now().Add(-48*time.Hour), time.Now().Add(-time.Hour)
	writeTranscriptAt(t, SessionsFolder(dir, "/p/museum"), "a"+transcriptExt, older)
	writeTranscriptAt(t, SessionsFolder(dir, "/p/museum"), "b"+transcriptExt, newer)
	writeTranscriptAt(t, SessionsFolder(dir, "/p/museum"), "notes.txt", time.Now())
	writeTranscriptAt(t, SessionsFolder(dir, "/p/museum-web"), "c"+transcriptExt, time.Now())

	got := LastActiveIn(dir, []string{"/p/museum", "/p/weather"})
	if len(got) != 1 || !got["/p/museum"].Equal(newer) {
		t.Errorf("LastActiveIn = %v, want only /p/museum at %v", got, newer)
	}
}

func writeTranscriptAt(t *testing.T, folder, name string, at time.Time) {
	t.Helper()
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(folder, name)
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, at, at); err != nil {
		t.Fatal(err)
	}
}
