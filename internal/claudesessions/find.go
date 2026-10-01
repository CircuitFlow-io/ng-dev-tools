package claudesessions

import (
	"context"
	"os"
	"path/filepath"
	"slices"

	"golang.org/x/sync/errgroup"
)

const (
	configDirVariable = "CLAUDE_CONFIG_DIR"
	defaultConfigDir  = ".claude"
	projectsDirName   = "projects"
	// maxParallelReads is small because one transcript line can hold megabytes of tool output.
	maxParallelReads = 4
)

// Dir is where Claude Code keeps its sessions: $CLAUDE_CONFIG_DIR/projects, or ~/.claude/projects.
func Dir(home string, getenv func(string) string) string {
	return filepath.Join(configDir(home, getenv), projectsDirName)
}

func configDir(home string, getenv func(string) string) string {
	if dir := getenv(configDirVariable); dir != "" {
		return dir
	}
	return filepath.Join(home, defaultConfigDir)
}

// FindAll reads every session in dir, most recently active first. Sessions without a prompt,
// such as one opened and closed straight away, are left out, and so are subagents' transcripts.
// A transcript that cannot be read is reported in errs by path and left out.
func FindAll(ctx context.Context, dir string) (sessions []Session, errs map[string]error, err error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, nil, err
	}
	paths, err := filepath.Glob(filepath.Join(dir, "*", "*"+transcriptExt))
	if err != nil {
		return nil, nil, err
	}
	read := make([]Session, len(paths))
	failed := make([]error, len(paths))
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(maxParallelReads)
	for i, path := range paths {
		g.Go(func() error {
			if gctx.Err() == nil {
				read[i], failed[i] = Read(path)
			}
			return nil
		})
	}
	_ = g.Wait()
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}

	errs = map[string]error{}
	for i, err := range failed {
		if err != nil {
			errs[paths[i]] = err
		}
	}
	sessions = slices.DeleteFunc(read, func(s Session) bool { return s.Prompts == 0 })
	Sort(sessions)
	return sessions, errs, nil
}
