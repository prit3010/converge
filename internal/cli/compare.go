package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/prit3010/converge/internal/llm"
	"github.com/spf13/cobra"
)

const compareTimeout = 45 * time.Second

func newCompareCmd() *cobra.Command {
	var model string
	var maxDiffLines int
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "compare <cellA> <cellB>",
		Short: "Use AI to summarize semantic differences between two cells",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runCompare(cwd, args[0], args[1], model, maxDiffLines, outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "OpenAI model to use (default gpt-4o-mini)")
	cmd.Flags().IntVar(&maxDiffLines, "max-diff-lines", 800, "Maximum diff lines to send to the model")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runCompare(projectDir, cellA, cellB, model string, maxDiffLines int, outputJSON bool, out io.Writer) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	comparer := llm.NewComparer(svc.DB, svc.Store)
	if !outputJSON {
		fmt.Fprintf(out, "Comparing %s -> %s ...\n\n", cellA, cellB)
	}
	ctx, cancel := context.WithTimeout(context.Background(), compareTimeout)
	defer cancel()

	result, err := comparer.Compare(ctx, cellA, cellB, llm.CompareOptions{
		Model:        strings.TrimSpace(model),
		MaxDiffLines: maxDiffLines,
	})
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("compare timed out after %s", compareTimeout)
		}
		if result != nil && result.Error != "" {
			return externalErrorf("compare failed: %s", result.Error)
		}
		return err
	}
	if outputJSON {
		return writeCommandSuccessJSON(out, "compare", map[string]any{
			"cell_a":         cellA,
			"cell_b":         cellB,
			"model":          strings.TrimSpace(model),
			"max_diff_lines": maxDiffLines,
			"result":         result,
		})
	}

	fmt.Fprintf(out, "Summary:\n  %s\n\n", strings.TrimSpace(result.Summary))
	if len(result.Highlights) > 0 {
		fmt.Fprintln(out, "Highlights:")
		for _, highlight := range result.Highlights {
			fmt.Fprintf(out, "  - %s\n", highlight)
		}
		fmt.Fprintln(out)
	}
	if strings.TrimSpace(result.Winner) != "" {
		fmt.Fprintf(out, "Winner: %s\n", result.Winner)
	}
	return nil
}
