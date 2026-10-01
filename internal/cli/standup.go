package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/standup"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/standup/tui"
)

type standupFlags struct {
	root  string
	since string
}

func newStandupCmd() *cobra.Command {
	var flags standupFlags
	cmd := &cobra.Command{
		Use:   "standup",
		Short: "What you did since your last working day, across your projects and GitHub",
		Long: `Show what you did since the last day you worked: the commits you wrote in every git repository
in ~/projects, grouped by project and then by the pull request or branch they belong to, the pull
requests you opened or merged, the others' ones you reviewed or commented on, and the work still
in progress (uncommitted changes and commits not pushed yet).

The last day you worked is the most recent day before today with a commit of yours, so a weekend
or a holiday is skipped. --since picks another start: today, yesterday, a weekday such as monday
for the week so far, a date, or a number of days or weeks back.

Press enter or o to open the commit, pull request or branch under the cursor on GitHub, and [ or ]
to start a day earlier or later. Commits are yours when their author is the user.email of their
repository. GitHub is read through the gh CLI; without it the report only has local work.

When output is not a terminal, the report is printed instead; --json prints it as JSON.`,
		Example: "  ngt standup\n  ngt standup --since monday\n  ngt standup --since 3d | pbcopy\n  ngt standup --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runStandup(cmd.Context(), cmd.OutOrStdout(), resolveOutput(cmd), flags)
		},
	}
	cmd.Flags().StringVar(&flags.root, "root", "", "folder that holds your projects (default: the projectsDir setting, ~/projects)")
	cmd.Flags().StringVar(&flags.since, "since", "", "start of the report: today, yesterday, monday, 2026-09-28, 3d or 2w (default the last day you worked)")
	return cmd
}

func runStandup(ctx context.Context, out io.Writer, mode outputMode, flags standupFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := projectsRoot(ctx, flags.root, home)
	var since time.Time
	if flags.since != "" {
		if since, err = standup.ParseSince(flags.since, time.Now()); err != nil {
			return err
		}
	}
	runner := macos.ExecRunner{}
	load := func(ctx context.Context, since time.Time) (standup.Report, error) {
		return standup.Load(ctx, runner, root, since, time.Now())
	}
	switch mode {
	case outputText:
		return printStandup(ctx, out, load, since)
	case outputJSON:
		report, err := load(ctx, since)
		if err != nil {
			return err
		}
		return writeJSON(out, toStandupJSON(report))
	}

	cfg := tui.Config{
		Root:    root,
		Home:    home,
		Since:   since,
		Load:    load,
		OpenURL: browserOpener(ctx, runner),
	}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	return final.(tui.Model).Err()
}

// printStandup prints the report as plain text, for when output is not a terminal.
func printStandup(ctx context.Context, out io.Writer, load func(context.Context, time.Time) (standup.Report, error), since time.Time) error {
	report, err := load(ctx, since)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, strings.Join(tui.PlainLines(report, time.Now()), "\n"))
	return err
}
