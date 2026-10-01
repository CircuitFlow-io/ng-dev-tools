package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions/tui"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/macos"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/pulls"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

// plainPromptWidth keeps the first prompt of printed sessions to one readable line.
const plainPromptWidth = 80

func newClaudeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "claude",
		Short: "Tools for Claude Code",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newClaudeSessionsCmd())
	return cmd
}

func newClaudeSessionsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "sessions [query]",
		Aliases: []string{"session"},
		Short:   "Every Claude Code session across your projects; search them and resume one",
		Long: `List every saved Claude Code session, from every folder, most recently active first, with its
first prompt, last activity, status, git branch, number of prompts and model. The status says
what Claude is doing with a session that is open: working, waiting on you (such as for a
permission) or idle after finishing its turn. Closed sessions have none. Type to search everything
said in them (your prompts and Claude's replies) and their titles, folders and branches; the
details box shows where the search matched.

Press enter to resume the session with claude --resume, in the folder it belongs to.

Sessions are read from ~/.claude/projects, or $CLAUDE_CONFIG_DIR/projects. When output is not a
terminal, the sessions are printed instead; --json prints them as JSON, tokens included.`,
		Example: "  ngt claude sessions\n  ngt claude sessions expo upgrade\n  ngt claude sessions | grep memorit\n  ngt claude sessions expo --json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClaudeSessions(cmd.Context(), cmd.OutOrStdout(), resolveOutput(cmd), strings.Join(args, " "))
		},
	}
}

func runClaudeSessions(ctx context.Context, out io.Writer, mode outputMode, query string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := claudesessions.Dir(home, os.Getenv)
	find := func(ctx context.Context) ([]claudesessions.Session, map[string]error, error) {
		return claudesessions.FindAll(ctx, dir)
	}
	liveDir := claudesessions.LiveDir(home, os.Getenv)
	live := func(ctx context.Context) map[string]claudesessions.Live {
		return claudesessions.ReadLive(ctx, liveDir, claudesessions.PSStartTimes)
	}
	switch mode {
	case outputText:
		return printClaudeSessions(ctx, out, find, live, query, home)
	case outputJSON:
		results, errs, err := searchSessions(ctx, find, query)
		if err != nil {
			return err
		}
		return writeJSON(out, toSessionsJSON(results, live(ctx), errs))
	}

	lookUpPRs := func(ctx context.Context, urls []string) (map[string]pulls.Summary, error) {
		return pulls.Lookup(ctx, macos.ExecRunner{}, urls)
	}
	cfg := tui.Config{
		Dir: dir, Home: home, Root: projectsRoot(ctx, "", home), Query: query,
		Find: find, Live: live, PRs: lookUpPRs, OpenURL: browserOpener(ctx, macos.ExecRunner{}),
	}
	final, err := tea.NewProgram(tui.New(ctx, cfg)).Run()
	if err != nil {
		return err
	}
	m := final.(tui.Model)
	if err := m.Err(); err != nil {
		return err
	}
	session, ok := m.Chosen()
	if !ok {
		return nil
	}
	lipgloss.Fprintln(out, ui.Selected.Render("▸ ")+"Resuming "+ui.Bold.Render(sessionName(session))+" in "+ui.TildePath(session.Dir, home))
	return claudesessions.Resume(session)
}

func sessionName(s claudesessions.Session) string {
	if s.Title != "" {
		return s.Title
	}
	return ui.Truncate(s.FirstPrompt, plainPromptWidth)
}

// printClaudeSessions lists the sessions matching query as plain text, for when output is not a
// terminal.
func printClaudeSessions(
	ctx context.Context,
	out io.Writer,
	find func(context.Context) ([]claudesessions.Session, map[string]error, error),
	live func(context.Context) map[string]claudesessions.Live,
	query, home string,
) error {
	results, errs, err := searchSessions(ctx, find, query)
	if err != nil {
		return err
	}
	if err := writeClaudeSessions(out, results, live(ctx), time.Now(), home); err != nil {
		return err
	}
	for _, path := range slices.Sorted(maps.Keys(errs)) {
		fmt.Fprintf(out, "%s: could not read: %v\n", ui.TildePath(path, home), errs[path])
	}
	return nil
}

func searchSessions(
	ctx context.Context,
	find func(context.Context) ([]claudesessions.Session, map[string]error, error),
	query string,
) ([]claudesessions.Result, map[string]error, error) {
	sessions, errs, err := find(ctx)
	if err != nil {
		return nil, nil, err
	}
	return claudesessions.NewIndex(sessions).Search(query), errs, nil
}

func writeClaudeSessions(out io.Writer, results []claudesessions.Result, live map[string]claudesessions.Live, now time.Time, home string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ACTIVE\tSTATUS\tFOLDER\tBRANCH\tPROMPTS\tMODEL\tSESSION\tFIRST PROMPT")
	for _, r := range results {
		s := r.Session
		fmt.Fprintln(w, strings.Join([]string{
			ui.Ago(now, s.LastActive),
			live[s.ID].Activity.String(),
			ui.TildePath(s.Dir, home),
			s.Branch,
			strconv.Itoa(s.Prompts),
			claudesessions.ModelName(s.MainModel()),
			s.ID,
			ui.Truncate(s.FirstPrompt, plainPromptWidth),
		}, "\t"))
	}
	return w.Flush()
}
