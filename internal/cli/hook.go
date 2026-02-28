package cli

import "github.com/spf13/cobra"

func newHookCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "hook",
		Short: "Hook entrypoints for agent integrations",
	}
	cmd.AddCommand(newHookCompleteCmd())
	return cmd
}
