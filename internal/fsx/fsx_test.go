package fsx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const blockAlignedSize = 64 * 1024

func writeFile(t *testing.T, path string, size int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiskUsageCountsHardLinksOnceAndSkipsSymlinkTargets(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeFile(t, filepath.Join(root, "a.bin"), blockAlignedSize)
	writeFile(t, filepath.Join(outside, "big.bin"), 4*blockAlignedSize)
	if err := os.Link(filepath.Join(root, "a.bin"), filepath.Join(root, "a-link.bin")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "outside")); err != nil {
		t.Fatal(err)
	}

	got := DiskUsage(context.Background(), root)

	if got < blockAlignedSize || got >= 2*blockAlignedSize {
		t.Errorf("DiskUsage = %d, want one copy of the %d byte file plus directory overhead", got, blockAlignedSize)
	}
}

func TestDiskUsageOfMissingPathIsZero(t *testing.T) {
	if got := DiskUsage(context.Background(), filepath.Join(t.TempDir(), "missing")); got != 0 {
		t.Errorf("DiskUsage = %d, want 0", got)
	}
}

func TestRemoveAllHandlesReadOnlyDirectories(t *testing.T) {
	root := filepath.Join(t.TempDir(), "modcache")
	writeFile(t, filepath.Join(root, "pkg", "file.go"), 10)
	if err := os.Chmod(filepath.Join(root, "pkg"), 0o555); err != nil {
		t.Fatal(err)
	}

	if err := RemoveAll(root); err != nil {
		t.Fatalf("RemoveAll: %v", err)
	}
	if Exists(root) {
		t.Error("directory still exists")
	}
}

func TestNeedsRootIsFalseForOwnFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mine")
	writeFile(t, path, 1)
	if NeedsRoot(path) {
		t.Error("NeedsRoot = true for a file we own")
	}
}

func TestNewestModTimeIgnoresSkippedEntries(t *testing.T) {
	project := t.TempDir()
	old := time.Now().Add(-100 * 24 * time.Hour)
	writeFile(t, filepath.Join(project, "main.go"), 1)
	writeFile(t, filepath.Join(project, "node_modules", "x.js"), 1)
	for _, p := range []string{filepath.Join(project, "main.go"), project} {
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}

	got := NewestModTime(project, map[string]bool{"node_modules": true})

	if !got.Equal(old) {
		t.Errorf("NewestModTime = %v, want %v", got, old)
	}
}
