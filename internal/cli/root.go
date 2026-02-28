package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "converge",
		Short:         "Local-first experiment tracker for AI coding",
		Long:          "Converge captures each AI-coding iteration as reproducible cells so you can compare, restore, and iterate quickly.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.AddCommand(newInitCmd())
	cmd.AddCommand(newSnapCmd())
	cmd.AddCommand(newEvalCmd())
	cmd.AddCommand(newLogCmd())
	cmd.AddCommand(newStatusCmd())
	cmd.AddCommand(newDiffCmd())
	cmd.AddCommand(newRestoreCmd())
	cmd.AddCommand(newWatchCmd())
	cmd.AddCommand(newForkCmd())
	cmd.AddCommand(newSwitchCmd())
	cmd.AddCommand(newBranchesCmd())
	cmd.AddCommand(newCompareCmd())
	cmd.AddCommand(newHookCmd())
	cmd.AddCommand(newUICmd())

	return cmd
}

func Execute() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
