package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/prittamravi/converge/internal/llm"
	"github.com/spf13/cobra"
)

const compareTimeout = 45 * time.Second

func newCompareCmd() *cobra.Command {
	var model string
	var maxDiffLines int
	cmd := &cobra.Command{
		Use:   "compare <cellA> <cellB>",
		Short: "Use AI to summarize semantic differences between two cells",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runCompare(cwd, args[0], args[1], model, maxDiffLines)
		},
	}
	cmd.Flags().StringVar(&model, "model", "", "OpenAI model to use (default gpt-4o-mini)")
	cmd.Flags().IntVar(&maxDiffLines, "max-diff-lines", 800, "Maximum diff lines to send to the model")
	return cmd
}

func runCompare(projectDir, cellA, cellB, model string, maxDiffLines int) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	comparer := llm.NewComparer(svc.DB, svc.Store)
	fmt.Printf("Comparing %s -> %s ...\n\n", cellA, cellB)
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
			return fmt.Errorf("compare failed: %s", result.Error)
		}
		return err
	}

	fmt.Printf("Summary:\n  %s\n\n", strings.TrimSpace(result.Summary))
	if len(result.Highlights) > 0 {
		fmt.Println("Highlights:")
		for _, highlight := range result.Highlights {
			fmt.Printf("  - %s\n", highlight)
		}
		fmt.Println()
	}
	if strings.TrimSpace(result.Winner) != "" {
		fmt.Printf("Winner: %s\n", result.Winner)
	}
	return nil
}
