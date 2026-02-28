package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/prittamravi/converge/internal/core"
	"github.com/spf13/cobra"
)

func newSnapCmd() *cobra.Command {
	var message string
	var tags string
	var agent string
	var runEval bool

	cmd := &cobra.Command{
		Use:   "snap",
		Short: "Create a new experiment cell from the current working tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSnap(cwd, message, tags, agent, runEval)
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "Cell message")
	cmd.Flags().StringVar(&tags, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent identifier")
	cmd.Flags().BoolVar(&runEval, "eval", true, "Run evaluation after snapshot")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

func runSnap(projectDir, message, tags, agent string, runEval bool) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	cell, err := svc.CreateCell(context.Background(), core.SnapOptions{
		Message: strings.TrimSpace(message),
		Tags:    tags,
		Agent:   agent,
		Source:  "manual",
		RunEval: runEval,
	})
	if err != nil {
		return fmt.Errorf("create cell: %w", err)
	}

	fmt.Printf("Created %s: %q\n", cell.ID, cell.Message)
	fmt.Printf("  Branch: %s\n", cell.Branch)
	fmt.Printf("  Files: %d (+%d ~%d -%d)  Lines: +%d/-%d\n", cell.TotalFiles, cell.FilesAdded, cell.FilesModified, cell.FilesRemoved, cell.LinesAdded, cell.LinesRemoved)
	fmt.Printf("  LOC total: %d  delta: %+d\n", cell.TotalLOC, cell.LOCDelta)
	if runEval {
		if cell.EvalRan {
			fmt.Println("  Eval: completed")
		} else {
			fmt.Println("  Eval: requested (see converge eval for details)")
		}
	}
	return nil
}
