package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "eval <cell>",
		Short: "Run on-demand evaluation for a cell",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runEval(cwd, args[0])
		},
	}
}

func runEval(projectDir, cellID string) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	result, err := svc.EvaluateCell(context.Background(), cellID)
	if err != nil {
		return err
	}

	fmt.Printf("Eval updated for %s\n", cellID)
	if result.HasTests {
		fmt.Printf("  Tests: %d passed, %d failed\n", result.TestsPassed, result.TestsFailed)
	}
	if result.HasLint {
		fmt.Printf("  Lint errors: %d\n", result.LintErrors)
	}
	if result.HasTypes {
		fmt.Printf("  Type errors: %d\n", result.TypeErrors)
	}
	if len(result.Skipped) > 0 {
		fmt.Printf("  Skipped: %s\n", strings.Join(result.Skipped, ", "))
	}
	return nil
}
