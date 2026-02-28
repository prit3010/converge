package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newForkCmd() *cobra.Command {
	var switchNow bool
	cmd := &cobra.Command{
		Use:   "fork <name>",
		Short: "Create a named branch from the current branch head",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runFork(cwd, args[0], switchNow)
		},
	}
	cmd.Flags().BoolVar(&switchNow, "switch", false, "Switch to the new branch immediately")
	return cmd
}

func runFork(projectDir, branchName string, switchNow bool) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	branch, err := svc.ForkBranch(strings.TrimSpace(branchName), switchNow)
	if err != nil {
		return err
	}

	head := "<none>"
	if branch.HeadCellID != nil {
		head = *branch.HeadCellID
	}
	fmt.Printf("Created branch %q at %s\n", branch.Name, head)
	if switchNow {
		fmt.Printf("Switched to %q\n", branch.Name)
	}
	return nil
}
