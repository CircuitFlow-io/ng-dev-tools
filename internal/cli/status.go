package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/gitstatus/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const maxParallelFetches = 8

type statusFlags struct {
	root  string
	fetch bool
}

func newStatusCmd() *cobra.Command {
	var flags statusFlags
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show the git state of every project: changes, unpushed commits, stashes and more",
		Long: `List the git repositories in ~/projects, the ones needing attention first: a merge or rebase
left in progress, a branch behind its remote, uncommitted changes, then commits not pushed yet.
The details box shows the changed files, the unpushed commits, the stashes and the other local
branches that are behind, not pushed or whose remote branch was deleted.

It only reads, and never contacts the remotes unless asked: --fetch, or f and F in the list, run
git fetch, which updates the remote-tracking branches and nothing else. Press enter to open a
repository in its IDE.

When output is not a terminal, the table is printed instead; --json prints everything as JSON.`,
		Example: "  ngt status\n  ngt status --fetch\n  ngt status | grep -v clean\n  ngt status --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode := resolveOutput(cmd)
			return runStatus(cmd.Context(), cmd.OutOrStdout(), noticeWriter(cmd, mode), mode, flags)
		},
	}
	cmd.Flags().StringVar(&flags.root, "root", "", "folder that holds your projects (default ~/projects)")
	cmd.Flags().BoolVar(&flags.fetch, "fetch", false, "git fetch every repository first, to know what is behind")
	return cmd
}

func runStatus(ctx context.Context, out, notices io.Writer, mode outputMode, flags statusFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := flags.root
	if root == "" {
		root = filepath.Join(home, defaultProjectsDir)
	}
	runner := macos.ExecRunner{}
	switch mode {
	case outputText:
		return printStatus(ctx, out, runner, root, flags.fetch)
	case outputJSON:
		repos, fetchErrs, err := loadStatus(ctx, runner, root, flags.fetch)
		if err != nil {
			return err
		}
		return writeJSON(out, toStatusJSON(repos, fetchErrs))
	}

	store := projects.DefaultStore(home, os.Getenv)
	state, err := store.Load()
	if err != nil {
		lipgloss.Fprintln(notices, ui.Warning.Render("Ignoring unreadable "+ui.TildePath(store.Path, home)+": "+err.Error()))
	}
	cfg := tui.Config{
		Root:         root,
		Home:         home,
		Runner:       runner,
		Fetch:        gitstatus.Fetch,
		FetchOnStart: flags.fetch,
		IDEs:         ide.Detect(ide.SearchDirs(home)),
		ProjectIDEs:  state.ProjectIDEs,
		DefaultIDE:   state.IDE,
		Open:         ideOpener(ctx, runner, store, &state),
	}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	return final.(tui.Model).Err()
}

// ideOpener opens a project from the status screen and remembers the IDE, like ngt open.
// The screen opens one project at a time, so state is never written concurrently.
func ideOpener(ctx context.Context, runner macos.Runner, store projects.Store, state *projects.State) func(string, ide.IDE) error {
	return func(path string, editor ide.IDE) error {
		name := filepath.Base(path)
		if err := ide.Open(ctx, runner, editor, path); err != nil {
			return fmt.Errorf("could not open %s in %s: %w", name, editor.Name, err)
		}
		if err := rememberIDE(store, state, path, editor); err != nil {
			return fmt.Errorf("opened %s, but could not remember the IDE: %w", name, err)
		}
		return nil
	}
}

// printStatus prints the table as plain text, for when output is not a terminal.
func printStatus(ctx context.Context, out io.Writer, runner macos.Runner, root string, fetch bool) error {
	repos, fetchErrs, err := loadStatus(ctx, runner, root, fetch)
	if err != nil {
		return err
	}
	now := time.Now()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tBRANCH\tCHANGES\tSYNC\tLAST COMMIT\tNOTES")
	for _, r := range repos {
		fmt.Fprintln(w, strings.Join(tui.PlainRow(r, now, errText(fetchErrs[r.Path])), "\t"))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	for _, r := range repos {
		if err := fetchErrs[r.Path]; err != nil {
			fmt.Fprintf(out, "%s: fetch failed: %v\n", r.Name, err)
		}
	}
	return nil
}

// loadStatus reads every repository under root, after fetching them first when fetch is set, and
// returns why each fetch failed, by path.
func loadStatus(ctx context.Context, runner macos.Runner, root string, fetch bool) ([]gitstatus.Repo, map[string]error, error) {
	repos, err := gitstatus.LoadAll(ctx, runner, root)
	if err != nil || !fetch {
		return repos, map[string]error{}, err
	}
	fetchErrs := fetchAll(ctx, repos)
	repos, err = gitstatus.LoadAll(ctx, runner, root)
	return repos, fetchErrs, err
}

// fetchAll fetches the repositories that have a remote, a few at a time, and returns why each
// failed, by path.
func fetchAll(ctx context.Context, repos []gitstatus.Repo) map[string]error {
	var mu sync.Mutex
	failed := map[string]error{}
	var g errgroup.Group
	g.SetLimit(maxParallelFetches)
	for _, r := range repos {
		if !r.HasRemote {
			continue
		}
		g.Go(func() error {
			if err := gitstatus.Fetch(ctx, r.Path); err != nil {
				mu.Lock()
				failed[r.Path] = err
				mu.Unlock()
			}
			return nil
		})
	}
	_ = g.Wait()
	return failed
}
