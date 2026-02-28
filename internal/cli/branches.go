package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newBranchesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "branches",
		Short: "List branches and their head cells",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runBranches(cwd)
		},
	}
}

func runBranches(projectDir string) error {
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
		fmt.Println("No branches found.")
		return nil
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
		fmt.Printf("%s%-20s head: %s\n", marker, b.Name, head)
	}
	return nil
}
