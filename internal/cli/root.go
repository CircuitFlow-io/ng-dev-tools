// Package cli defines ngt's commands and flags.
package cli

import "github.com/spf13/cobra"

// NewRootCmd builds the ngt command tree. New features register their subcommand here.
func NewRootCmd(version string) *cobra.Command {
	root := &cobra.Command{
		Use:          "ngt",
		Short:        "A personal toolbox of developer utilities",
		Version:      version,
		SilenceUsage: true,
	}
	root.AddCommand(newCleanCmd(), newPortsCmd(), newDoctorCmd(), newOpenCmd(), newRunCmd(), newStatusCmd(), newPRsCmd(), newEnvCmd(), newTodoCmd(), newStandupCmd())
	return root
}
