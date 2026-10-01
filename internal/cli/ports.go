package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ports/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const maxPort = 65535

var (
	errNeedsConfirmation     = errors.New("stdin is not a terminal: pass --yes to stop without confirming")
	errJSONNeedsConfirmation = errors.New("--json cannot ask before stopping: pass --yes to stop without confirming")
)

type portsFlags struct {
	all   bool
	force bool
	yes   bool
	grace time.Duration
}

func newPortsCmd() *cobra.Command {
	var flags portsFlags
	cmd := &cobra.Command{
		Use:   "ports [port...]",
		Short: "List processes listening on TCP ports and stop the ones you pick",
		Long: `Without arguments, show every process listening on a TCP port with its project folder and
uptime, and stop the ones you pick. With port numbers, stop whatever is listening on them.

Processes get SIGTERM so they can shut down cleanly, and SIGKILL if they are still running
after the grace period. macOS system processes are hidden unless --all is given.

--json prints the listening processes, or with port numbers and --yes what was stopped, as JSON.`,
		Example: "  ngt ports\n  ngt ports 3000\n  ngt ports 3000 8080 --yes\n  ngt ports | grep node\n  ngt ports --json\n  ngt ports 3000 --yes --json",
		Args:    validPorts,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPorts(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), resolveOutput(cmd), args, flags)
		},
	}

	f := cmd.Flags()
	f.BoolVar(&flags.all, "all", false, "include macOS system processes")
	f.BoolVarP(&flags.force, "force", "f", false, "send SIGKILL straight away instead of SIGTERM first")
	f.BoolVarP(&flags.yes, "yes", "y", false, "stop the processes on the given ports without asking")
	f.DurationVar(&flags.grace, "grace", ports.DefaultGrace, "how long to wait after SIGTERM before sending SIGKILL")
	return cmd
}

func validPorts(_ *cobra.Command, args []string) error {
	_, err := parsePorts(args)
	return err
}

func parsePorts(args []string) ([]int, error) {
	numbers := make([]int, 0, len(args))
	for _, arg := range args {
		port, err := strconv.Atoi(strings.TrimPrefix(arg, ":"))
		if err != nil || port < 1 || port > maxPort {
			return nil, fmt.Errorf("invalid port %q", arg)
		}
		numbers = append(numbers, port)
	}
	return numbers, nil
}

func runPorts(ctx context.Context, in io.Reader, out io.Writer, mode outputMode, args []string, flags portsFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	runner := macos.ExecRunner{}
	lister := ports.Lister{Runner: runner, Home: home}
	stopper := ports.Stopper{Runner: runner, Grace: flags.grace, Force: flags.force}

	if len(args) > 0 {
		wanted, err := parsePorts(args)
		if err != nil {
			return err
		}
		if mode == outputJSON {
			return stopPortsJSON(ctx, out, lister, stopper, wanted, flags.yes)
		}
		confirm := confirmer(in, out, flags.yes)
		return stopPorts(ctx, out, lister, stopper, wanted, confirm)
	}
	switch mode {
	case outputText:
		return printPorts(ctx, out, lister, flags.all)
	case outputJSON:
		processes, err := lister.List(ctx)
		if err != nil {
			return err
		}
		return writeJSON(out, toPortsJSON(visibleProcesses(processes, flags.all), time.Now()))
	}

	final, err := tea.NewProgram(tui.New(ctx, tui.Config{Lister: lister, Stopper: stopper, ShowSystem: flags.all})).Run()
	if err != nil {
		return err
	}
	return final.(tui.Model).Err()
}

// stopPorts stops whatever listens on wanted after confirm approves the list.
func stopPorts(ctx context.Context, out io.Writer, lister ports.Lister, stopper ports.Stopper, wanted []int, confirm func(prompt string) (bool, error)) error {
	processes, err := lister.List(ctx)
	if err != nil {
		return err
	}
	holders, free := ports.Holding(processes, wanted)
	for _, port := range free {
		lipgloss.Fprintln(out, ui.Muted.Render(fmt.Sprintf("Nothing is listening on :%d", port)))
	}
	if len(holders) == 0 {
		return nil
	}

	for _, p := range holders {
		lipgloss.Fprintln(out, describeHolder(p, lister.Home))
	}
	approved, err := confirm(fmt.Sprintf("Stop %s? [y/N] ", ui.Count(len(holders), "process")))
	if err != nil || !approved {
		return err
	}

	results := stopper.StopAll(ctx, holders, nil)
	lipgloss.Fprintln(out, tui.Results(results))
	return stopFailure(results)
}

// stopPortsJSON stops whatever listens on wanted without asking, which assumeYes must allow, and
// reports it as JSON.
func stopPortsJSON(ctx context.Context, out io.Writer, lister ports.Lister, stopper ports.Stopper, wanted []int, assumeYes bool) error {
	if !assumeYes {
		return errJSONNeedsConfirmation
	}
	processes, err := lister.List(ctx)
	if err != nil {
		return err
	}
	holders, free := ports.Holding(processes, wanted)
	results := stopper.StopAll(ctx, holders, nil)
	if err := writeJSON(out, toStopJSON(free, results, time.Now())); err != nil {
		return err
	}
	return stopFailure(results)
}

func stopFailure(results []ports.StopResult) error {
	if failed := countFailed(results); failed > 0 {
		return fmt.Errorf("could not stop %s", ui.Count(failed, "process"))
	}
	return nil
}

func describeHolder(p ports.Process, home string) string {
	line := ui.Bold.Render(p.Label()) + ui.Muted.Render("  "+ui.TildePath(p.Project, home))
	if caution := p.Caution(); caution != "" {
		line += "\n" + ui.Warning.Render("  "+caution)
	}
	return line
}

// confirmer asks on in, unless the user already agreed with --yes.
func confirmer(in io.Reader, out io.Writer, assumeYes bool) func(string) (bool, error) {
	return func(prompt string) (bool, error) {
		if assumeYes {
			return true, nil
		}
		if f, ok := in.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) {
			return false, errNeedsConfirmation
		}
		fmt.Fprint(out, prompt)
		answer, err := bufio.NewReader(in).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		return slices.Contains([]string{"y", "yes"}, strings.ToLower(strings.TrimSpace(answer))), nil
	}
}

func countFailed(results []ports.StopResult) int {
	var failed int
	for _, r := range results {
		if r.Err != nil {
			failed++
		}
	}
	return failed
}

// printPorts lists listening processes as plain text, for when output is not a terminal.
func printPorts(ctx context.Context, out io.Writer, lister ports.Lister, all bool) error {
	processes, err := lister.List(ctx)
	if err != nil {
		return err
	}
	now := time.Now()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PORTS\tPID\tPROCESS\tUPTIME\tPROJECT\tADDRESSES")
	for _, p := range visibleProcesses(processes, all) {
		fmt.Fprintf(w, "%s\t%d\t%s\t%s\t%s\t%s\n", p.PortList(), p.PID, p.Name, ui.Elapsed(p.Uptime(now)),
			ui.TildePath(p.Project, lister.Home), strings.Join(p.Addresses, ","))
	}
	return w.Flush()
}

// visibleProcesses leaves out macOS system and simulator processes unless all is set.
func visibleProcesses(processes []ports.Process, all bool) []ports.Process {
	if all {
		return processes
	}
	return slices.DeleteFunc(slices.Clone(processes), ports.Process.IsSystem)
}
