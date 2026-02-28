package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newRestoreCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restore <cell>",
		Short: "Restore tracked files to a target cell state",
		Long:  "Creates a safety snapshot first, then restores tracked files from the target cell while leaving untracked files untouched.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runRestore(cwd, args[0])
		},
	}
}

func runRestore(projectDir, targetID string) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	safety, err := svc.RestoreCell(context.Background(), targetID)
	if err != nil {
		return err
	}
	fmt.Printf("Created safety cell: %s\n", safety.ID)
	fmt.Printf("Restored working tree to %s\n", targetID)
	return nil
}
