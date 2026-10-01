package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/envfiles/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
)

type envFlags struct {
	root string
}

func newEnvCmd() *cobra.Command {
	var flags envFlags
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Compare each project's .env files with their examples, by key name only",
		Long: `Compare the env files of every project in ~/projects with the example that documents them
(.env.example, .env.sample, .env.template), folder by folder, monorepo apps included. It shows the
keys your files are missing, the ones they leave empty, the ones the example does not list, and the
keys the code reads that no env file has. Local env files that git tracks, or that are still in its
history after being deleted, come first: their secrets may have leaked.

Only key names are shown, never values. Press a to append the missing keys to the local file with
empty values; nothing else is ever written.

When output is not a terminal, the table is printed instead; --json prints every key name as JSON.`,
		Example: "  ngt env\n  ngt env | grep tracked\n  ngt env --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runEnvCheck(cmd.Context(), cmd.OutOrStdout(), resolveOutput(cmd), flags)
		},
	}
	cmd.Flags().StringVar(&flags.root, "root", "", "folder that holds your projects (default ~/projects)")
	return cmd
}

func runEnvCheck(ctx context.Context, out io.Writer, mode outputMode, flags envFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := flags.root
	if root == "" {
		root = filepath.Join(home, defaultProjectsDir)
	}
	runner := macos.ExecRunner{}
	scan := func(ctx context.Context) ([]envfiles.Set, error) { return envfiles.ScanAll(ctx, runner, root) }
	if mode != outputTUI {
		sets, err := scan(ctx)
		if err != nil {
			return err
		}
		if mode == outputJSON {
			return writeJSON(out, toEnvJSON(sets))
		}
		return writeEnv(out, sets)
	}
	cfg := tui.Config{Root: root, Home: home, Scan: scan, AddMissing: envfiles.AddMissing}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	return final.(tui.Model).Err()
}

func writeEnv(out io.Writer, sets []envfiles.Set) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "FOLDER\tFILES\tMISSING\tEXTRA\tEMPTY\tNOTES")
	for _, s := range sets {
		fmt.Fprintln(w, strings.Join(tui.PlainRow(s), "\t"))
	}
	return w.Flush()
}
