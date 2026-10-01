package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

type prsFlags struct {
	root string
}

func newPRsCmd() *cobra.Command {
	var flags prsFlags
	cmd := &cobra.Command{
		Use:     "prs",
		Aliases: []string{"pr"},
		Short:   "Your open pull requests and the ones waiting for your review, across GitHub",
		Long: `List the pull requests waiting for your review and your own open ones, across every repository
on GitHub, with their CI, review and merge state. The details box shows each check, each
reviewer's verdict and whether the branch conflicts with its base.

Press enter to open a pull request in the browser, c to check out its branch in your local clone
(found in ~/projects by its remote; refused while that clone has uncommitted changes, and cloned
into ~/projects with gh repo clone first when there is none), i to check
out its branch the same way and open the clone in its IDE, and l to read the log of its failed GitHub Actions checks.

It reads GitHub through the gh CLI, so it uses gh's login. When output is not a terminal, the pull
requests are printed instead; --json prints everything, checks and reviews included, as JSON.`,
		Example: "  ngt prs\n  ngt prs | grep -i conflicts\n  ngt prs --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode := resolveOutput(cmd)
			return runPRs(cmd.Context(), cmd.OutOrStdout(), noticeWriter(cmd, mode), mode, flags)
		},
	}
	cmd.Flags().StringVar(&flags.root, "root", "", "folder that holds your local clones (default: the projectsDir setting, ~/projects)")
	return cmd
}

func runPRs(ctx context.Context, out, notices io.Writer, mode outputMode, flags prsFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := projectsRoot(ctx, flags.root, home)
	runner := macos.ExecRunner{}
	switch mode {
	case outputText:
		return printPRs(ctx, out, runner)
	case outputJSON:
		d, err := pulls.Load(ctx, runner)
		if err != nil {
			return err
		}
		return writeJSON(out, toPRsJSON(d))
	}

	store := projects.DefaultStore(home, os.Getenv)
	state, err := store.Load()
	if err != nil {
		lipgloss.Fprintln(notices, ui.Warning.Render("Ignoring unreadable "+ui.TildePath(store.Path, home)+": "+err.Error()))
	}
	cfg := tui.Config{
		Root:   root,
		Home:   home,
		Load:   func(ctx context.Context) (pulls.Dashboard, error) { return pulls.Load(ctx, runner) },
		Clones: func() map[string]string { return pulls.Clones(root) },
		Clone: func(ctx context.Context, repo string) (string, error) {
			return pulls.Clone(ctx, root, repo)
		},
		Checkout: func(ctx context.Context, dir string, p pulls.PR) error {
			return pulls.Checkout(ctx, runner, dir, p)
		},
		FailedLog: func(ctx context.Context, check pulls.Check) ([]pulls.LogLine, error) {
			return pulls.FailedLog(ctx, runner, check)
		},
		OpenURL:     browserOpener(ctx, runner),
		IDEs:        ide.Detect(ide.SearchDirs(home)),
		ProjectIDEs: state.ProjectIDEs,
		DefaultIDE:  state.IDE,
		Open:        ideOpener(ctx, runner, store, &state),
	}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	return final.(tui.Model).Err()
}

// printPRs lists the pull requests as plain text, for when output is not a terminal.
func printPRs(ctx context.Context, out io.Writer, runner macos.Runner) error {
	d, err := pulls.Load(ctx, runner)
	if err != nil {
		return err
	}
	return writePRs(out, d, time.Now())
}

func writePRs(out io.Writer, d pulls.Dashboard, now time.Time) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "GROUP\tPULL REQUEST\tTITLE\tSTATE\tCI\tUPDATED\tURL")
	for _, group := range []struct {
		name string
		prs  []pulls.PR
	}{{"review", d.ToReview}, {"mine", d.Mine}} {
		for _, p := range group.prs {
			fmt.Fprintln(w, group.name+"\t"+strings.Join(tui.PlainRow(p, d.Viewer, now), "\t"))
		}
	}
	return w.Flush()
}
