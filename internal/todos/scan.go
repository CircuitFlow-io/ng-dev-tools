package todos

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	maxParallelFiles = 4
	// maxFileBytes skips files too big to be written by hand, such as data and generated code.
	maxFileBytes = 1 << 20
	// maxLineBytes skips minified code, where a marker is not a note anyone left.
	maxLineBytes = 400
	// binaryProbeBytes is how much of a file git checks for a NUL byte to tell it is binary.
	binaryProbeBytes = 8000
)

// excludedPaths leave out dependencies, build output and generated files that are committed anyway.
var excludedPaths = []string{
	":(exclude,glob)**/node_modules/**", ":(exclude,glob)**/vendor/**", ":(exclude,glob)**/dist/**",
	":(exclude,glob)**/build/**", ":(exclude,glob)**/Pods/**", ":(exclude,glob)**/*.min.js",
	":(exclude,glob)**/*.map", ":(exclude,glob)**/*lock.json", ":(exclude,glob)**/*lock.yaml", ":(exclude,glob)**/*.lock",
}

var markerWords = [][]byte{[]byte(Todo), []byte(Fixme), []byte(Hack)}

// findMarkers reads the marker comments in the tracked files and the untracked ones git does not
// ignore. The files are read here rather than by git grep, which reads each file whole and prints
// each matching line whole, however big.
func findMarkers(ctx context.Context, runner macos.Runner, project, dir string) ([]Item, error) {
	files, err := listFiles(ctx, runner, dir)
	if err != nil {
		return nil, err
	}
	found := make([][]Item, len(files))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelFiles)
	for i, file := range files {
		g.Go(func() error {
			found[i] = markersIn(dir, file)
			return gctx.Err()
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	items := slices.Concat(found...)
	for i := range items {
		items[i].Project, items[i].Dir = project, dir
	}
	return items, nil
}

func listFiles(ctx context.Context, runner macos.Runner, dir string) ([]string, error) {
	args := append([]string{"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--deduplicate", "--", "."}, excludedPaths...)
	out, err := git(ctx, runner, dir, args...)
	if err != nil {
		return nil, err
	}
	return strings.FieldsFunc(string(out), func(r rune) bool { return r == 0 }), nil
}

// markersIn reads the marker comments in one file of the repository in dir. A file that is binary,
// too big or not a regular file, such as a symlink or a named pipe, has none.
func markersIn(dir, file string) []Item {
	data, ok := readSmallFile(filepath.Join(dir, file))
	if !ok || isBinary(data) || !mentionsMarker(data) {
		return nil
	}
	var items []Item
	number := 0
	for line := range bytes.Lines(data) {
		number++
		text := bytes.TrimSuffix(line, []byte("\n"))
		if len(text) > maxLineBytes || !mentionsMarker(text) {
			continue
		}
		if marker, note, ok := parseMarker(file, string(text)); ok {
			items = append(items, Item{File: file, Line: number, Marker: marker, Note: note})
		}
	}
	return items
}

// readSmallFile reads a regular file of at most maxFileBytes. It checks the file before opening it,
// since opening a named pipe waits for a writer.
func readSmallFile(path string) ([]byte, bool) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxFileBytes {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxFileBytes+1))
	return data, err == nil && len(data) <= maxFileBytes
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), binaryProbeBytes)], 0) >= 0
}

func mentionsMarker(text []byte) bool {
	return slices.ContainsFunc(markerWords, func(word []byte) bool { return bytes.Contains(text, word) })
}
