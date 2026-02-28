package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show status relative to latest cell",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runStatus(cwd)
		},
	}
}

func runStatus(projectDir string) error {
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
	if latest == nil {
		fmt.Printf("Active branch: %s\n", activeBranch)
		fmt.Println("No cells yet. Run 'converge snap -m \"message\"' to create one.")
		return nil
	}

	headCellID := ""
	if branchRecord.HeadCellID != nil {
		headCellID = *branchRecord.HeadCellID
	}
	fmt.Printf("Active branch: %s", activeBranch)
	if headCellID != "" {
		fmt.Printf(" (head %s)", headCellID)
	}
	fmt.Println()
	fmt.Printf("Last cell: [%s] %s %q\n", latest.ID, latest.Timestamp, latest.Message)
	fmt.Printf("  branch: %s\n", latest.Branch)
	fmt.Printf("  complexity(LOC): %d (delta %+d)\n", latest.TotalLOC, latest.LOCDelta)
	if delta.Modified+delta.Added+delta.Removed == 0 {
		fmt.Println("  Working tree is clean (matches last cell)")
	} else {
		fmt.Printf("  Changes since last cell: %d modified, %d new, %d deleted\n", delta.Modified, delta.Added, delta.Removed)
	}
	return nil
}
