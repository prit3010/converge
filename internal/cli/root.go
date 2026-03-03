package cli

import (
	"fmt"
	"os"
	"strings"

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
	cmd.AddCommand(newGitHooksCmd())
	cmd.AddCommand(newArchivesCmd())
	cmd.AddCommand(newUICmd())

	return cmd
}

func Execute() {
	root := NewRootCmd()
	executedCmd, err := root.ExecuteC()
	if err == nil {
		return
	}

	cmdErr := classifyCommandError(err)
	if commandWantsJSON(executedCmd) || argsRequestJSON(os.Args[1:]) {
		command := "converge"
		if executedCmd != nil && strings.TrimSpace(executedCmd.Name()) != "" {
			command = executedCmd.Name()
		}
		if writeErr := writeCommandErrorJSON(os.Stdout, command, cmdErr); writeErr != nil {
			fmt.Fprintln(os.Stderr, writeErr)
		}
		os.Exit(cmdErr.ExitCode)
	}

	fmt.Fprintln(os.Stderr, err)
	os.Exit(cmdErr.ExitCode)
}

func commandWantsJSON(cmd *cobra.Command) bool {
	if cmd == nil {
		return false
	}
	flag := cmd.Flags().Lookup("json")
	if flag == nil {
		return false
	}
	value, err := cmd.Flags().GetBool("json")
	return err == nil && value
}

func argsRequestJSON(args []string) bool {
	for _, arg := range args {
		if arg == "--json" {
			return true
		}
	}
	return false
}
