package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prittamravi/converge/internal/core"
	"github.com/prittamravi/converge/internal/snapshot"
	"github.com/spf13/cobra"
)

func newSnapCmd() *cobra.Command {
	var message string
	var tags string
	var agent string
	var runEval bool
	var outputJSON bool

	cmd := &cobra.Command{
		Use:   "snap",
		Short: "Create a new experiment cell from the current working tree",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runSnap(cwd, message, tags, agent, runEval, outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVarP(&message, "message", "m", "", "Cell message")
	cmd.Flags().StringVar(&tags, "tags", "", "Comma-separated tags")
	cmd.Flags().StringVar(&agent, "agent", "", "Agent identifier")
	cmd.Flags().BoolVar(&runEval, "eval", true, "Run evaluation after snapshot")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	_ = cmd.MarkFlagRequired("message")
	return cmd
}

type snapJSON struct {
	Cell    any                   `json:"cell"`
	Skipped []snapshot.SkipReason `json:"skipped,omitempty"`
}

func runSnap(projectDir, message, tags, agent string, runEval bool, outputJSON bool, out io.Writer) error {
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
	skipped := svc.LastSnapshotSkipped()
	if outputJSON {
		payload := snapJSON{
			Cell:    cell,
			Skipped: skipped,
		}
		return writeCommandSuccessJSON(out, "snap", payload)
	}

	fmt.Fprintf(out, "Created %s: %q\n", cell.ID, cell.Message)
	fmt.Fprintf(out, "  Branch: %s\n", cell.Branch)
	fmt.Fprintf(out, "  Files: %d (+%d ~%d -%d)  Lines: +%d/-%d\n", cell.TotalFiles, cell.FilesAdded, cell.FilesModified, cell.FilesRemoved, cell.LinesAdded, cell.LinesRemoved)
	fmt.Fprintf(out, "  LOC total: %d  delta: %+d\n", cell.TotalLOC, cell.LOCDelta)
	if runEval {
		if cell.EvalRan {
			fmt.Fprintln(out, "  Eval: completed")
		} else {
			fmt.Fprintln(out, "  Eval: requested (see converge eval for details)")
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintln(out, "  Skipped by policy:")
		for _, item := range skipped {
			fmt.Fprintf(out, "    - %s (%s)\n", item.Path, item.Reason)
		}
	}
	return nil
}
