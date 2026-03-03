package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newBranchesCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "branches",
		Short: "List branches and their head cells",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runBranches(cwd, outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runBranches(projectDir string, outputJSON bool, out io.Writer) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	activeBranch, err := svc.ActiveBranch()
	if err != nil {
		return err
	}

	branches, err := svc.DB.ListBranches()
	if err != nil {
		return err
	}
	if len(branches) == 0 {
		if outputJSON {
			return writeCommandSuccessJSON(out, "branches", map[string]any{
				"active_branch": activeBranch,
				"branches":      []any{},
			})
		}
		fmt.Fprintln(out, "No branches found.")
		return nil
	}
	if outputJSON {
		return writeCommandSuccessJSON(out, "branches", map[string]any{
			"active_branch": activeBranch,
			"branches":      branches,
		})
	}

	for _, b := range branches {
		marker := "  "
		if b.Name == activeBranch {
			marker = "* "
		}
		head := "<none>"
		if b.HeadCellID != nil {
			head = *b.HeadCellID
		}
		fmt.Fprintf(out, "%s%-20s head: %s\n", marker, b.Name, head)
	}
	return nil
}
