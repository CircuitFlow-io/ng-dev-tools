package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"

	"github.com/CircuitFlow-io/ng-dev-tools/internal/settings"
	"github.com/CircuitFlow-io/ng-dev-tools/internal/ui"
)

const notSet = "not set"

type settingsKey struct{}

// loadSettings reads the settings before any command runs, links ticket keys to the Jira host and
// keeps the settings in the command's context for projectsRoot.
func loadSettings(cmd *cobra.Command, _ []string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	store := settings.DefaultStore(home, os.Getenv)
	loaded, err := store.Load()
	if err != nil {
		lipgloss.Fprintln(cmd.ErrOrStderr(), ui.Warning.Render("Ignoring unreadable "+ui.TildePath(store.Path, home)+": "+err.Error()))
	}
	ui.LinkTickets(loaded.TicketURLPrefix())
	cmd.SetContext(context.WithValue(cmd.Context(), settingsKey{}, loaded))
}

func settingsFrom(ctx context.Context) settings.Settings {
	loaded, _ := ctx.Value(settingsKey{}).(settings.Settings)
	return loaded
}

// projectsRoot is the folder given with --root, or else the projects-dir setting.
func projectsRoot(ctx context.Context, flagRoot, home string) string {
	if flagRoot != "" {
		return flagRoot
	}
	return settingsFrom(ctx).ProjectsRoot(home)
}

func newSettingsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "settings",
		Short: "Show and change ngt's settings",
		Long: `List ngt's settings with their values, or change one with set and unset.

Settings:
  projects-dir  folder that holds your projects (default ~/projects). Every command with a --root
                flag uses it when --root is not given.
  jira-host     Jira site, such as acme.atlassian.net. Ticket keys like TS-1234 in branch names,
                commit subjects, pull request titles and TODO notes become links to their page,
                clickable in terminals that support links (cmd+click in iTerm2, Ghostty, WezTerm).

The settings are kept in ~/.config/ngt/settings.json.`,
		Example: "  ngt settings\n  ngt settings set projects-dir ~/work\n  ngt settings set jira-host acme.atlassian.net\n" +
			"  ngt settings unset jira-host\n  cd \"$(ngt settings get projects-dir)\"",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSettingsList(cmd.OutOrStdout(), resolveOutput(cmd))
		},
	}
	cmd.AddCommand(newSettingsGetCmd(), newSettingsSetCmd(), newSettingsUnsetCmd())
	return cmd
}

func newSettingsGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "get <setting>",
		Short:     "Print a setting's value, or its default when it is not set",
		Args:      cobra.ExactArgs(1),
		ValidArgs: settingNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettingsGet(cmd.OutOrStdout(), resolveOutput(cmd), args[0])
		},
	}
}

func newSettingsSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "set <setting> <value>",
		Short:     "Change a setting",
		Args:      cobra.ExactArgs(2),
		ValidArgs: settingNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettingsSet(cmd.OutOrStdout(), resolveOutput(cmd), args[0], args[1])
		},
	}
}

func newSettingsUnsetCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "unset <setting>",
		Short:     "Put a setting back to its default",
		Args:      cobra.ExactArgs(1),
		ValidArgs: settingNames(),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSettingsUnset(cmd.OutOrStdout(), resolveOutput(cmd), args[0])
		},
	}
}

func settingNames() []string {
	names := make([]string, 0, len(settings.Keys))
	for _, k := range settings.Keys {
		names = append(names, k.Name)
	}
	return names
}

// settingsEnv is what the settings subcommands work with: the store and the settings it holds.
type settingsEnv struct {
	home   string
	store  settings.Store
	loaded settings.Settings
}

// openSettings reads the settings file, failing on an unreadable one so a change never replaces it.
func openSettings() (settingsEnv, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return settingsEnv{}, err
	}
	store := settings.DefaultStore(home, os.Getenv)
	loaded, err := store.Load()
	if err != nil {
		return settingsEnv{}, fmt.Errorf("could not read %s: %w", ui.TildePath(store.Path, home), err)
	}
	return settingsEnv{home: home, store: store, loaded: loaded}, nil
}

func runSettingsList(out io.Writer, mode outputMode) error {
	env, err := openSettings()
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return writeJSON(out, toSettingsJSON(env.store.Path, env.loaded, env.home))
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "SETTING\tVALUE\tDESCRIPTION")
	for _, k := range settings.Keys {
		fmt.Fprintf(w, "%s\t%s\t%s\n", k.Name, displayValue(k, env.loaded, env.home), k.Description)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if mode == outputTUI {
		lipgloss.Fprintln(out, ui.Help.Render("Change one with ngt settings set <setting> <value>. Kept in "+ui.TildePath(env.store.Path, env.home)+"."))
	}
	return nil
}

// displayValue is the setting's value for people: home as ~, and marked when it is the default.
func displayValue(k settings.Key, s settings.Settings, home string) string {
	value := ui.TildePath(k.Value(s, home), home)
	switch {
	case value == "":
		return notSet
	case !k.IsSet(s):
		return value + " (default)"
	}
	return value
}

func runSettingsGet(out io.Writer, mode outputMode, name string) error {
	k, err := settings.Lookup(name)
	if err != nil {
		return err
	}
	env, err := openSettings()
	if err != nil {
		return err
	}
	if mode == outputJSON {
		return writeJSON(out, toSettingJSON(k, env.loaded, env.home))
	}
	_, err = fmt.Fprintln(out, k.Value(env.loaded, env.home))
	return err
}

func runSettingsSet(out io.Writer, mode outputMode, name, value string) error {
	k, err := settings.Lookup(name)
	if err != nil {
		return err
	}
	env, err := openSettings()
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := k.Set(&env.loaded, value, settings.Env{Home: env.home, Dir: cwd}); err != nil {
		return err
	}
	return env.save(out, mode, k, "Set "+k.Name+" to "+ui.TildePath(k.Value(env.loaded, env.home), env.home))
}

func runSettingsUnset(out io.Writer, mode outputMode, name string) error {
	k, err := settings.Lookup(name)
	if err != nil {
		return err
	}
	env, err := openSettings()
	if err != nil {
		return err
	}
	k.Unset(&env.loaded)
	done := "Unset " + k.Name
	if k.Default != "" {
		done = "Reset " + k.Name + " to its default, " + k.Default
	}
	return env.save(out, mode, k, done)
}

// save writes the settings and reports the changed setting k, with done for people.
func (e settingsEnv) save(out io.Writer, mode outputMode, k settings.Key, done string) error {
	if err := e.store.Save(e.loaded); err != nil {
		return err
	}
	if mode == outputJSON {
		return writeJSON(out, toSettingJSON(k, e.loaded, e.home))
	}
	_, err := lipgloss.Fprintln(out, ui.Success.Render("✓ ")+done)
	return err
}
