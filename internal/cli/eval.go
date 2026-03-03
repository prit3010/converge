package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newEvalCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "eval <cell>",
		Short: "Run on-demand evaluation for a cell",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runEval(cwd, args[0], outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runEval(projectDir, cellID string, outputJSON bool, out io.Writer) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	result, err := svc.EvaluateCell(context.Background(), cellID)
	if err != nil {
		return err
	}
	if outputJSON {
		return writeCommandSuccessJSON(out, "eval", map[string]any{
			"cell_id":       cellID,
			"tests_passed":  result.TestsPassed,
			"tests_failed":  result.TestsFailed,
			"lint_errors":   result.LintErrors,
			"type_errors":   result.TypeErrors,
			"has_tests":     result.HasTests,
			"has_lint":      result.HasLint,
			"has_types":     result.HasTypes,
			"skipped":       result.Skipped,
			"used_override": svc.Policy.Eval.HasOverrides(),
		})
	}

	fmt.Fprintf(out, "Eval updated for %s\n", cellID)
	if result.HasTests {
		fmt.Fprintf(out, "  Tests: %d passed, %d failed\n", result.TestsPassed, result.TestsFailed)
	}
	if result.HasLint {
		fmt.Fprintf(out, "  Lint errors: %d\n", result.LintErrors)
	}
	if result.HasTypes {
		fmt.Fprintf(out, "  Type errors: %d\n", result.TypeErrors)
	}
	if len(result.Skipped) > 0 {
		fmt.Fprintf(out, "  Skipped: %s\n", strings.Join(result.Skipped, ", "))
	}
	return nil
}
