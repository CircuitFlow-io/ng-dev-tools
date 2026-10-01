package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/dustin/go-humanize"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup/rules"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/cleanup/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

const (
	day              = 24 * time.Hour
	week             = 7 * day
	defaultOlderThan = "90d"
	defaultMinSize   = "100MB"
	logDirPerm       = 0o755
	logTimeLayout    = "20060102-150405"
)

// commonProjectDirs are searched for stale build artifacts besides the projectsDir setting.
var commonProjectDirs = []string{"Developer", "code", "src", "workspace"}

type cleanFlags struct {
	dryRun      bool
	olderThan   string
	categories  []string
	projectDirs []string
	minSize     string
}

func newCleanCmd() *cobra.Command {
	var flags cleanFlags
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Find and permanently remove caches, developer junk, unused apps and old files",
		Long: `Scan your Mac for reclaimable disk space, pick what to remove in an interactive list,
and delete it permanently. Items marked "review" are not selected by default.

Categories:
  caches  system, browser and app caches, logs, Trash, iOS updates and backups
  dev     Xcode, simulators, package manager caches, Docker, stale project build folders
  apps    applications not opened recently and leftovers from uninstalled apps
  files   large or installer files in Downloads and Desktop not opened recently

When output is not a terminal, or with --json, what was found is printed and nothing is deleted.`,
		Example: "  ngt clean\n  ngt clean --dry-run\n  ngt clean --category dev --older-than 30d\n  ngt clean --category dev --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runClean(cmd.Context(), cmd.OutOrStdout(), resolveOutput(cmd), flags)
		},
	}

	f := cmd.Flags()
	f.BoolVar(&flags.dryRun, "dry-run", false, "go through the whole flow without deleting anything")
	f.StringVar(&flags.olderThan, "older-than", defaultOlderThan, "treat things unused for this long as stale (e.g. 30d, 12w, 720h)")
	f.StringSliceVar(&flags.categories, "category", categoryKeys(), "categories to scan")
	f.StringSliceVar(&flags.projectDirs, "projects-dir", nil, "folders searched for stale project build artifacts (default: the projectsDir setting, ~/Developer, ~/code, ~/src, ~/workspace)")
	f.StringVar(&flags.minSize, "min-size", defaultMinSize, "minimum size of a large old file")
	return cmd
}

func runClean(ctx context.Context, out io.Writer, mode outputMode, flags cleanFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	env, categories, err := flags.env(home, settingsFrom(ctx).ProjectsRoot(home))
	if err != nil {
		return err
	}
	scanner := cleanup.Scanner{Rules: rules.ForCategories(rules.Default(), categories), Env: env}

	switch mode {
	case outputText:
		return printReport(ctx, out, scanner)
	case outputJSON:
		return writeJSON(out, toCleanJSON(scanner.Scan(ctx, nil)))
	}

	logFile, logPath, err := openLog(home, env.Now)
	if err != nil {
		return err
	}
	defer logFile.Close()

	model := tui.New(ctx, tui.Config{
		Scanner: scanner,
		Cleaner: cleanup.Cleaner{Guard: cleanup.NewGuard(home), Runner: env.Runner, DryRun: flags.dryRun, Log: logFile},
		LogPath: logPath,
	})
	_, err = tea.NewProgram(model).Run()
	return err
}

func (f cleanFlags) env(home, projectsRoot string) (cleanup.Env, []cleanup.Category, error) {
	staleAfter, err := parseAge(f.olderThan)
	if err != nil {
		return cleanup.Env{}, nil, fmt.Errorf("--older-than: %w", err)
	}
	minSize, err := humanize.ParseBytes(f.minSize)
	if err != nil {
		return cleanup.Env{}, nil, fmt.Errorf("--min-size: %w", err)
	}
	categories, err := parseCategories(f.categories)
	if err != nil {
		return cleanup.Env{}, nil, fmt.Errorf("--category: %w", err)
	}
	env := cleanup.Env{
		Home:             home,
		Now:              time.Now(),
		StaleAfter:       staleAfter,
		ProjectDirs:      projectDirs(home, projectsRoot, f.projectDirs),
		LargeFileMinSize: int64(minSize),
		Runner:           macos.ExecRunner{},
	}
	return env, categories, nil
}

// parseAge accepts Go durations plus day ("90d") and week ("12w") suffixes.
func parseAge(value string) (time.Duration, error) {
	units := map[string]time.Duration{"d": day, "w": week}
	for suffix, unit := range units {
		if number, ok := strings.CutSuffix(value, suffix); ok {
			n, err := strconv.Atoi(number)
			if err != nil || n <= 0 {
				return 0, fmt.Errorf("invalid age %q", value)
			}
			return time.Duration(n) * unit, nil
		}
	}
	d, err := time.ParseDuration(value)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("invalid age %q", value)
	}
	return d, nil
}

func parseCategories(keys []string) ([]cleanup.Category, error) {
	categories := make([]cleanup.Category, 0, len(keys))
	for _, key := range keys {
		c, err := cleanup.ParseCategory(key)
		if err != nil {
			return nil, err
		}
		categories = append(categories, c)
	}
	return categories, nil
}

func categoryKeys() []string {
	keys := make([]string, 0, len(cleanup.AllCategories))
	for _, c := range cleanup.AllCategories {
		keys = append(keys, c.Key())
	}
	return keys
}

func projectDirs(home, projectsRoot string, given []string) []string {
	if len(given) > 0 {
		return given
	}
	dirs := []string{projectsRoot}
	for _, name := range commonProjectDirs {
		if dir := filepath.Join(home, name); dir != projectsRoot {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

func openLog(home string, now time.Time) (*os.File, string, error) {
	dir := filepath.Join(home, "Library", "Logs", "ngt")
	if err := os.MkdirAll(dir, logDirPerm); err != nil {
		return nil, "", err
	}
	path := filepath.Join(dir, "clean-"+now.Format(logTimeLayout)+".log")
	f, err := os.Create(path)
	return f, path, err
}
