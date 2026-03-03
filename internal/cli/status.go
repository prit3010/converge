package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show status relative to latest cell",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runStatus(cwd, outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runStatus(projectDir string, outputJSON bool, out io.Writer) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	activeBranch, err := svc.ActiveBranch()
	if err != nil {
		return err
	}
	branchRecord, err := svc.DB.GetBranch(activeBranch)
	if err != nil {
		return err
	}

	latest, delta, err := svc.WorkingTreeDelta(context.Background())
	if err != nil {
		return err
	}
	headCellID := ""
	if branchRecord.HeadCellID != nil {
		headCellID = *branchRecord.HeadCellID
	}
	if latest == nil {
		if outputJSON {
			return writeCommandSuccessJSON(out, "status", map[string]any{
				"active_branch": activeBranch,
				"head_cell_id":  headCellID,
				"has_cells":     false,
				"clean":         true,
				"delta":         delta,
			})
		}
		fmt.Fprintf(out, "Active branch: %s\n", activeBranch)
		fmt.Fprintln(out, "No cells yet. Run 'converge snap -m \"message\"' to create one.")
		return nil
	}
	if outputJSON {
		return writeCommandSuccessJSON(out, "status", map[string]any{
			"active_branch": activeBranch,
			"head_cell_id":  headCellID,
			"has_cells":     true,
			"latest_cell":   latest,
			"delta":         delta,
			"clean":         delta.Modified+delta.Added+delta.Removed == 0,
		})
	}

	fmt.Fprintf(out, "Active branch: %s", activeBranch)
	if headCellID != "" {
		fmt.Fprintf(out, " (head %s)", headCellID)
	}
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Last cell: [%s] %s %q\n", latest.ID, latest.Timestamp, latest.Message)
	fmt.Fprintf(out, "  branch: %s\n", latest.Branch)
	fmt.Fprintf(out, "  complexity(LOC): %d (delta %+d)\n", latest.TotalLOC, latest.LOCDelta)
	if delta.Modified+delta.Added+delta.Removed == 0 {
		fmt.Fprintln(out, "  Working tree is clean (matches last cell)")
	} else {
		fmt.Fprintf(out, "  Changes since last cell: %d modified, %d new, %d deleted\n", delta.Modified, delta.Added, delta.Removed)
	}
	return nil
}
