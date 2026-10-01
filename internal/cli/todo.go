package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/ide"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/todos/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

type todoFlags struct {
	root string
	mine bool
}

func newTodoCmd() *cobra.Command {
	var flags todoFlags
	cmd := &cobra.Command{
		Use:     "todo",
		Aliases: []string{"todos"},
		Short:   "TODO, FIXME and HACK comments across your projects, oldest first",
		Long: `List the TODO, FIXME and HACK comments in every git repository in ~/projects, dated with git
blame so the longest forgotten come first. Only markers in comments count, so a string such as
'TODO' in code is not one. Tracked files and untracked ones git does not ignore are searched;
dependencies and build output are left out.

Press enter to open the file at that line in your IDE, o to open the commit that added it on
GitHub, and m to show only the lines you wrote. A line is yours when its author has any
user.email or user.name set for its repository, or your GitHub profile name or noreply address.

When output is not a terminal, the comments are printed instead; --json prints everything, blame
included, as JSON.`,
		Example: "  ngt todo\n  ngt todo --mine\n  ngt todo | grep FIXME\n  ngt todo --mine --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			mode := resolveOutput(cmd)
			return runTodo(cmd.Context(), cmd.OutOrStdout(), noticeWriter(cmd, mode), mode, flags)
		},
	}
	cmd.Flags().StringVar(&flags.root, "root", "", "folder that holds your projects (default: the projectsDir setting, ~/projects)")
	cmd.Flags().BoolVar(&flags.mine, "mine", false, "only print the lines you wrote, when output is not a terminal or with --json")
	return cmd
}

func runTodo(ctx context.Context, out, notices io.Writer, mode outputMode, flags todoFlags) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	root := projectsRoot(ctx, flags.root, home)
	runner := macos.ExecRunner{}
	find := func(ctx context.Context) ([]todos.Item, map[string]error, error) {
		return todos.FindAll(ctx, runner, root)
	}
	switch mode {
	case outputText:
		return printTodos(ctx, out, find, flags.mine)
	case outputJSON:
		items, errs, err := findTodos(ctx, find, flags.mine)
		if err != nil {
			return err
		}
		return writeJSON(out, toTodosJSON(items, errs))
	}

	store := projects.DefaultStore(home, os.Getenv)
	state, err := store.Load()
	if err != nil {
		lipgloss.Fprintln(notices, ui.Warning.Render("Ignoring unreadable "+ui.TildePath(store.Path, home)+": "+err.Error()))
	}
	cfg := tui.Config{
		Root:        root,
		Home:        home,
		Find:        find,
		OpenURL:     browserOpener(ctx, runner),
		IDEs:        ide.Detect(ide.SearchDirs(home)),
		ProjectIDEs: state.ProjectIDEs,
		DefaultIDE:  state.IDE,
		OpenAt:      lineOpener(ctx, runner, store, &state),
	}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	return final.(tui.Model).Err()
}

// lineOpener opens a comment's file at its line and remembers the IDE for the project, like ngt
// open. The screen opens one file at a time, so state is never written concurrently.
func lineOpener(ctx context.Context, runner macos.Runner, store projects.Store, state *projects.State) func(todos.Item, ide.IDE) error {
	return func(item todos.Item, editor ide.IDE) error {
		file := filepath.Join(item.Dir, item.File)
		if err := ide.OpenAt(ctx, runner, editor, item.Dir, file, item.Line); err != nil {
			return fmt.Errorf("could not open %s in %s: %w", item.Location(), editor.Name, err)
		}
		if err := rememberIDE(store, state, item.Dir, editor); err != nil {
			return fmt.Errorf("opened %s, but could not remember the IDE: %w", item.Location(), err)
		}
		return nil
	}
}

// printTodos lists the comments as plain text, for when output is not a terminal.
func printTodos(ctx context.Context, out io.Writer, find func(context.Context) ([]todos.Item, map[string]error, error), mine bool) error {
	items, errs, err := findTodos(ctx, find, mine)
	if err != nil {
		return err
	}
	if err := writeTodos(out, items, time.Now()); err != nil {
		return err
	}
	for _, name := range slices.Sorted(maps.Keys(errs)) {
		fmt.Fprintf(out, "%s: could not search: %v\n", name, errs[name])
	}
	return nil
}

// findTodos finds the comments, keeping only the lines you wrote when mine is set.
func findTodos(ctx context.Context, find func(context.Context) ([]todos.Item, map[string]error, error), mine bool) ([]todos.Item, map[string]error, error) {
	items, errs, err := find(ctx)
	if err != nil || !mine {
		return items, errs, err
	}
	return slices.DeleteFunc(items, func(i todos.Item) bool { return !i.Mine }), errs, nil
}

func writeTodos(out io.Writer, items []todos.Item, now time.Time) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "AGE\tMARKER\tPROJECT\tFILE\tAUTHOR\tNOTE")
	for _, item := range items {
		fmt.Fprintln(w, strings.Join(tui.PlainRow(item, now), "\t"))
	}
	return w.Flush()
}
