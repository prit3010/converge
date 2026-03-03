package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newRestoreCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "restore <cell>",
		Short: "Restore tracked files to a target cell state",
		Long:  "Creates a safety snapshot first, then restores tracked files from the target cell while leaving untracked files untouched.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runRestore(cwd, args[0], outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runRestore(projectDir, targetID string, outputJSON bool, out io.Writer) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	safety, err := svc.RestoreCell(context.Background(), targetID)
	if err != nil {
		return err
	}
	if outputJSON {
		return writeCommandSuccessJSON(out, "restore", map[string]any{
			"target_cell_id": targetID,
			"safety_cell_id": safety.ID,
		})
	}
	fmt.Fprintf(out, "Created safety cell: %s\n", safety.ID)
	fmt.Fprintf(out, "Restored working tree to %s\n", targetID)
	return nil
}
