package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/projects"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/scripts"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/scripts/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const rootPackageLabel = "."

var (
	errNotInProject = errors.New("not inside an npm or pnpm project")
	errNoRunsYet    = errors.New("no script has been run with ngt yet")
	errJSONRunsNone = errors.New("--json only lists scripts; run without it to use --last")
)

type runFlags struct {
	root string
	last bool
}

// runEnv is what running a script needs to know about the machine.
type runEnv struct {
	out     io.Writer
	home    string
	nvmDir  string
	store   scripts.HistoryStore
	history scripts.History
}

func newRunCmd() *cobra.Command {
	var flags runFlags
	cmd := &cobra.Command{
		Use:   "run [package] [script]",
		Short: "Pick a package.json script, in any package of a monorepo, and run it",
		Long: `Inside an npm or pnpm project, list its scripts: for a monorepo, the packages on the left and
the focused package's scripts on the right. Type to search every package, and press enter to run
the script in its package folder. The details box shows the full command, the scripts it calls,
pre/post hooks and the Node version it runs with (from .nvmrc, via nvm).

Outside a project, pick one from ~/projects first. The scripts you run are remembered and listed
first next time.

With arguments, a script named exactly by them runs straight away: "ngt run test" runs the
current package's test, "ngt run web dev" the dev script of apps/web. Otherwise the arguments
start the search.

When output is not a terminal, or with --json, the scripts are printed instead and nothing runs.`,
		Example: "  ngt run\n  ngt run web dev\n  ngt run --last\n  ngt run | grep test\n  ngt run --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			mode := resolveOutput(cmd)
			return runRun(cmd.Context(), cmd.OutOrStdout(), noticeWriter(cmd, mode), mode, args, flags)
		},
	}
	cmd.Flags().StringVar(&flags.root, "root", "", "folder that holds your projects (default: the projectsDir setting, ~/projects)")
	cmd.Flags().BoolVar(&flags.last, "last", false, "rerun the last script run in this project, or anywhere when outside one")
	return cmd
}

func runRun(ctx context.Context, out, notices io.Writer, mode outputMode, words []string, flags runFlags) error {
	if mode == outputJSON && flags.last {
		return errJSONRunsNone
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	env := runEnv{out: out, home: home, nvmDir: scripts.NVMDir(home, os.Getenv), store: scripts.DefaultHistoryStore(home, os.Getenv)}
	env.history, err = env.store.Load()
	if err != nil {
		lipgloss.Fprintln(notices, ui.Warning.Render("Ignoring unreadable "+ui.TildePath(env.store.Path, home)+": "+err.Error()))
	}

	ws, err := workspaceAt(cwd, home)
	if err != nil {
		return err
	}
	if flags.last {
		return env.runLast(ws)
	}
	if mode != outputTUI {
		if ws == nil {
			return errNotInProject
		}
		targets := listedScripts(*ws, words, env.history)
		if mode == outputJSON {
			return writeJSON(out, toScriptsJSON(*ws, targets))
		}
		return printScripts(out, targets)
	}
	if ws != nil {
		if t, ok := scripts.Exact(ws.Targets(), currentPackage(*ws, cwd), words); ok {
			return env.runScript(*ws, t)
		}
	}
	return env.pick(ctx, ws, cwd, words, flags.root)
}

// workspaceAt loads the project holding cwd, or returns nil when cwd is in none.
func workspaceAt(cwd, home string) (*scripts.Workspace, error) {
	root, ok := scripts.FindRoot(cwd, home)
	if !ok {
		return nil, nil
	}
	ws, err := scripts.Load(root)
	if err != nil {
		return nil, err
	}
	return &ws, nil
}

func currentPackage(ws scripts.Workspace, cwd string) string {
	pkg, _ := ws.PackageFor(cwd)
	return pkg.RelDir
}

func (e runEnv) pick(ctx context.Context, ws *scripts.Workspace, cwd string, words []string, root string) error {
	cfg := tui.Config{
		Root:       projectsRoot(ctx, root, e.home),
		Home:       e.home,
		Workspace:  ws,
		CurrentDir: cwd,
		Query:      strings.Join(words, " "),
		History:    e.history,
		Used:       e.lastUsed(),
		NVMDir:     e.nvmDir,
		Runner:     macos.ExecRunner{},
	}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	m := final.(tui.Model)
	if err := m.Err(); err != nil {
		return err
	}
	chosen, target, ok := m.Chosen()
	if !ok {
		return nil
	}
	return e.runScript(chosen, target)
}

// lastUsed merges when each project was opened with ngt open and when a script last ran in it.
func (e runEnv) lastUsed() map[string]time.Time {
	opened, _ := projects.DefaultStore(e.home, os.Getenv).Load()
	used := maps.Clone(opened.Opened)
	if used == nil {
		used = map[string]time.Time{}
	}
	for root, at := range e.history.LastRun() {
		if at.After(used[root]) {
			used[root] = at
		}
	}
	return used
}

func (e runEnv) runLast(ws *scripts.Workspace) error {
	if ws == nil {
		root, run, ok := e.history.LastAnywhere()
		if !ok {
			return errNoRunsYet
		}
		loaded, err := scripts.Load(root)
		if err != nil {
			return err
		}
		return e.rerun(loaded, run)
	}
	recent := e.history.Recent(ws.Root)
	if len(recent) == 0 {
		return fmt.Errorf("no script has been run in %s with ngt yet", ws.Name())
	}
	return e.rerun(*ws, recent[0])
}

func (e runEnv) rerun(ws scripts.Workspace, run scripts.Run) error {
	t, ok := ws.Find(run.Package, run.Script)
	if !ok {
		return fmt.Errorf("%s no longer has the script %s", ws.Name(), run.Script)
	}
	return e.runScript(ws, t)
}

// runScript records the run and replaces ngt with the script, so it only returns on failure.
func (e runEnv) runScript(ws scripts.Workspace, t scripts.Target) error {
	node := scripts.RequiredNode(t.Package.Dir, ws.Root, e.nvmDir)
	inv := scripts.Plan(ws, t, node, os.Environ())

	line := ui.Selected.Render("▸ ") + ui.Bold.Render(ws.Name()) + "  " + inv.Display
	if node.Installed() {
		line += ui.Muted.Render("  (Node " + node.Version + ")")
	}
	lipgloss.Fprintln(e.out, line)
	if node.Spec != "" && !node.Installed() {
		lipgloss.Fprintln(e.out, ui.Warning.Render(fmt.Sprintf("%s asks for Node %s, which nvm does not have; running with the current Node", node.Source, node.Spec)))
	}

	e.history.Record(ws.Root, scripts.Run{Package: t.Package.RelDir, Script: t.Script.Name, At: time.Now()})
	if err := e.store.Save(e.history); err != nil {
		lipgloss.Fprintln(e.out, ui.Warning.Render("Could not remember this run: "+err.Error()))
	}
	return scripts.Exec(inv)
}

// listedScripts is every script of ws, or the ones words search for.
func listedScripts(ws scripts.Workspace, words []string, history scripts.History) []scripts.Target {
	targets := ws.Targets()
	if len(words) == 0 {
		return targets
	}
	return scripts.Search(targets, history.Recent(ws.Root), strings.Join(words, " "))
}

// printScripts lists the scripts as plain text, for when output is not a terminal.
func printScripts(out io.Writer, targets []scripts.Target) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PACKAGE\tSCRIPT\tCOMMAND")
	for _, t := range targets {
		pkg := t.Package.RelDir
		if t.Package.IsRoot() {
			pkg = rootPackageLabel
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", pkg, t.Script.Name, t.Script.Command)
	}
	return w.Flush()
}
