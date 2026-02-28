package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newSwitchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "switch <branch>",
		Short: "Switch active branch and restore its head cell",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSwitch(cwd, args[0])
		},
	}
}

func runSwitch(projectDir, branchName string) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	safety, target, err := svc.SwitchBranch(context.Background(), strings.TrimSpace(branchName))
	if err != nil {
		return err
	}

	fmt.Printf("Switched to branch %q\n", target.Branch)
	fmt.Printf("Branch head: %s\n", target.ID)
	if safety != nil {
		fmt.Printf("Created safety cell: %s\n", safety.ID)
	}
	return nil
}
