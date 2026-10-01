package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/doctor"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/doctor/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

var errChecksFailed = errors.New("some checks failed")

type doctorFlags struct {
	offline  bool
	problems bool
}

func newDoctorCmd() *cobra.Command {
	var flags doctorFlags
	cmd := &cobra.Command{
		Use:   "doctor [group...]",
		Short: "Check that this Mac is ready for Node, React Native, iOS, Android and Go work",
		Long: `Check the tools, settings and health of this Mac for fullstack, mobile and Go development,
and print the command that fixes each problem. Doctor only reads; it never changes anything.

Groups: ` + strings.Join(doctor.GroupKeys(), ", ") + `

Exits with status 1 when any check fails, so it can be used in scripts. --json prints the
results as JSON, with the same exit status.`,
		Example:   "  ngt doctor\n  ngt doctor android go\n  ngt doctor --problems\n  ngt doctor --offline\n  ngt doctor --problems --json",
		ValidArgs: doctor.GroupKeys(),
		Args:      validGroups,
		RunE: func(cmd *cobra.Command, args []string) error {
			err := runDoctor(cmd.Context(), cmd.OutOrStdout(), resolveOutput(cmd), args, flags)
			if errors.Is(err, errChecksFailed) {
				// The report already explains what failed; only the exit status is left to set.
				cmd.SilenceErrors = true
			}
			return err
		},
	}

	f := cmd.Flags()
	f.BoolVar(&flags.offline, "offline", false, "skip checks and version lookups that need the network")
	f.BoolVarP(&flags.problems, "problems", "p", false, "only show warnings and failures")
	return cmd
}

func validGroups(_ *cobra.Command, args []string) error {
	_, err := parseGroups(args)
	return err
}

func parseGroups(args []string) ([]doctor.Group, error) {
	groups := make([]doctor.Group, 0, len(args))
	for _, arg := range args {
		group, err := doctor.ParseGroup(arg)
		if err != nil {
			return nil, err
		}
		groups = append(groups, group)
	}
	return groups, nil
}

func runDoctor(ctx context.Context, out io.Writer, mode outputMode, args []string, flags doctorFlags) error {
	groups, err := parseGroups(args)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	d := doctor.Doctor{
		Checks:  doctor.Select(doctor.Default(), groups),
		Env:     doctor.NewEnv(home),
		Offline: flags.offline,
	}

	if mode == outputJSON {
		outcomes := d.Run(ctx, nil)
		if err := writeJSON(out, toDoctorJSON(outcomes, flags.problems)); err != nil {
			return err
		}
		return failure(outcomes)
	}

	start := time.Now()
	outcomes, cancelled, err := examine(ctx, d, mode)
	if err != nil {
		return err
	}
	if cancelled {
		lipgloss.Fprintln(out, ui.Muted.Render("Cancelled."))
		return nil
	}
	report := tui.Report(outcomes, tui.ReportOptions{ProblemsOnly: flags.problems, Elapsed: time.Since(start)})
	lipgloss.Fprintln(out, "\n"+report)
	return failure(outcomes)
}

// examine runs the checks behind a progress screen when stdout is a terminal.
func examine(ctx context.Context, d doctor.Doctor, mode outputMode) (outcomes []doctor.Outcome, cancelled bool, err error) {
	if mode != outputTUI {
		return d.Run(ctx, nil), false, nil
	}
	final, err := tea.NewProgram(tui.New(ctx, d)).Run()
	if err != nil {
		return nil, false, err
	}
	m := final.(tui.Model)
	return m.Outcomes(), m.Cancelled(), nil
}

func failure(outcomes []doctor.Outcome) error {
	if failed := doctor.Count(outcomes, doctor.StatusFail); failed > 0 {
		return fmt.Errorf("%w: %d", errChecksFailed, failed)
	}
	return nil
}
