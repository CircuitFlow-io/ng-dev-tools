package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrintProjectsFiltersByQuery(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"museum", "home-app", "weather"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name, "package.json"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := printProjects(context.Background(), &out, root, "", "m", nil, openedVerb); err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 3 || !strings.HasPrefix(lines[0], "PROJECT") {
		t.Fatalf("output:\n%s", out.String())
	}
	if strings.Contains(out.String(), "weather") {
		t.Errorf("unmatched project printed:\n%s", out.String())
	}
}

func TestPrintProjectsNamesTheUseThatOrdersThem(t *testing.T) {
	root := t.TempDir()
	museum := filepath.Join(root, "museum")
	if err := os.MkdirAll(museum, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(museum, "package.json"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	used := map[string]time.Time{museum: time.Now().Add(time.Hour)}

	var out bytes.Buffer
	if err := printProjects(context.Background(), &out, root, "", "", used, claudeUsedVerb); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), " claude 1s ago ") {
		t.Errorf("activity not named after Claude:\n%s", out.String())
	}
}
