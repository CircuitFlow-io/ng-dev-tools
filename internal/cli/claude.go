package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/claudesessions/tui"
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
first prompt, last activity, git branch, number of prompts and model. Type to search everything
said in them (your prompts and Claude's replies) and their titles, folders and branches; the
details box shows where the search matched.

Press enter to resume the session with claude --resume, in the folder it belongs to.

Sessions are read from ~/.claude/projects, or $CLAUDE_CONFIG_DIR/projects. When output is not a
terminal, the sessions are printed instead.`,
		Example: "  ngt claude sessions\n  ngt claude sessions expo upgrade\n  ngt claude sessions | grep memorit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runClaudeSessions(cmd.Context(), cmd.OutOrStdout(), strings.Join(args, " "))
		},
	}
}

func runClaudeSessions(ctx context.Context, out io.Writer, query string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := claudesessions.Dir(home, os.Getenv)
	find := func(ctx context.Context) ([]claudesessions.Session, map[string]error, error) {
		return claudesessions.FindAll(ctx, dir)
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return printClaudeSessions(ctx, out, find, query, home)
	}

	cfg := tui.Config{Dir: dir, Home: home, Root: filepath.Join(home, defaultProjectsDir), Query: query, Find: find}
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
func printClaudeSessions(ctx context.Context, out io.Writer, find func(context.Context) ([]claudesessions.Session, map[string]error, error), query, home string) error {
	sessions, errs, err := find(ctx)
	if err != nil {
		return err
	}
	results := claudesessions.NewIndex(sessions).Search(query)
	if err := writeClaudeSessions(out, results, time.Now(), home); err != nil {
		return err
	}
	for _, path := range slices.Sorted(maps.Keys(errs)) {
		fmt.Fprintf(out, "%s: could not read: %v\n", ui.TildePath(path, home), errs[path])
	}
	return nil
}

func writeClaudeSessions(out io.Writer, results []claudesessions.Result, now time.Time, home string) error {
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ACTIVE\tFOLDER\tBRANCH\tPROMPTS\tMODEL\tSESSION\tFIRST PROMPT")
	for _, r := range results {
		s := r.Session
		fmt.Fprintln(w, strings.Join([]string{
			ui.Ago(now, s.LastActive),
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
