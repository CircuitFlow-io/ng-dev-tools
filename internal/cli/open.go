package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const defaultProjectsDir = "projects"

var errNoIDE = errors.New("no supported IDE found in /Applications or ~/Applications")

type openFlags struct {
	root string
}

func newOpenCmd() *cobra.Command {
	var flags openFlags
	cmd := &cobra.Command{
		Use:   "open [query]",
		Short: "Pick a project and open it in one of your IDEs",
		Long: `List the projects in ~/projects, most recently opened or changed first, with their git branch
and a ● for uncommitted changes. Type to filter, press enter, and pick the IDE to open the project
in. Each project remembers its IDE, and a project opened for the first time starts on the IDE
you picked most recently.

A folder that only groups other folders is replaced by the projects inside it. Xcode opens the
project's workspace (or its ios/ one), and Android Studio a React Native app's android/ folder.

When output is not a terminal, the projects are printed instead and nothing is opened.`,
		Example: "  ngt open\n  ngt open museum\n  ngt open --root ~/work\n  ngt open | grep weather",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runOpen(cmd.Context(), cmd.OutOrStdout(), strings.Join(args, ""), flags)
		},
	}
	cmd.Flags().StringVar(&flags.root, "root", "", "folder that holds your projects (default ~/projects)")
	return cmd
}

func runOpen(ctx context.Context, out io.Writer, query string, flags openFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := flags.root
	if root == "" {
		root = filepath.Join(home, defaultProjectsDir)
	}
	store := projects.DefaultStore(home, os.Getenv)
	state, err := store.Load()
	if err != nil {
		lipgloss.Fprintln(out, ui.Warning.Render("Ignoring unreadable "+ui.TildePath(store.Path, home)+": "+err.Error()))
	}

	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return printProjects(ctx, out, root, home, query, state)
	}

	ides := ide.Detect(ide.SearchDirs(home))
	if len(ides) == 0 {
		return errNoIDE
	}
	runner := macos.ExecRunner{}
	cfg := tui.Config{Root: root, Home: home, Query: query, State: state, IDEs: ides, Runner: runner}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	m := final.(tui.Model)
	if err := m.Err(); err != nil {
		return err
	}
	project, editor, ok := m.Chosen()
	if !ok {
		return nil
	}
	return openProject(ctx, out, runner, store, state, project, editor)
}

func openProject(ctx context.Context, out io.Writer, runner macos.Runner, store projects.Store, state projects.State, project projects.Project, editor ide.IDE) error {
	if err := ide.Open(ctx, runner, editor, project.Path); err != nil {
		return fmt.Errorf("could not open %s in %s: %w", project.Name, editor.Name, err)
	}
	lipgloss.Fprintln(out, "Opened "+ui.Bold.Render(project.Name)+" in "+ui.Bold.Render(editor.Name))

	if err := rememberIDE(store, &state, project.Path, editor); err != nil {
		lipgloss.Fprintln(out, ui.Warning.Render("Could not remember this choice: "+err.Error()))
	}
	return nil
}

// rememberIDE records that the project at path was opened in editor, so its IDE box starts there.
func rememberIDE(store projects.Store, state *projects.State, path string, editor ide.IDE) error {
	state.RecordOpen(path, editor.AppPath, time.Now())
	return store.Save(*state)
}

// printProjects lists the projects as plain text, for when output is not a terminal.
func printProjects(ctx context.Context, out io.Writer, root, home, query string, state projects.State) error {
	found, err := projects.Scan(ctx, root, state.Opened)
	if err != nil {
		return err
	}
	now := time.Now()
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PROJECT\tBRANCH\tLAST ACTIVITY\tPATH")
	for _, p := range projects.Filter(found, query) {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", p.Name, p.Branch, p.ActivityVerb()+" "+ui.Ago(now, p.LastActivity()), ui.TildePath(p.Path, home))
	}
	return w.Flush()
}
