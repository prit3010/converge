package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prittamravi/converge/internal/core"
	"github.com/spf13/cobra"
)

type hookCompleteFlags struct {
	RunID      string
	Agent      string
	Message    string
	Tags       string
	RunEval    bool
	OutputJSON bool
}

func newHookCompleteCmd() *cobra.Command {
	flags := &hookCompleteFlags{}
	cmd := &cobra.Command{
		Use:   "complete",
		Short: "Record an agent completion and auto-snapshot if changed",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runHookComplete(cwd, *flags, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&flags.RunID, "run-id", "", "Unique run identifier used for idempotency")
	cmd.Flags().StringVar(&flags.Agent, "agent", "", "Agent name (for example: codex, claude)")
	cmd.Flags().StringVarP(&flags.Message, "message", "m", "", "Agent completion message")
	cmd.Flags().StringVar(&flags.Tags, "tags", "", "CSV tags")
	cmd.Flags().BoolVar(&flags.RunEval, "eval", false, "Run evaluation after snapshot")
	cmd.Flags().BoolVar(&flags.OutputJSON, "json", false, "Print machine-readable JSON result")
	return cmd
}

func runHookComplete(projectDir string, flags hookCompleteFlags, out io.Writer) error {
	if err := validateHookCompleteFlags(flags); err != nil {
		return err
	}

	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	result, hookErr := svc.HandleAgentCompletion(context.Background(), core.AgentCompletionOptions{
		RunID:   strings.TrimSpace(flags.RunID),
		Agent:   strings.TrimSpace(flags.Agent),
		Message: strings.TrimSpace(flags.Message),
		Tags:    strings.TrimSpace(flags.Tags),
		RunEval: flags.RunEval,
	})
	if flags.OutputJSON {
		if err := writeJSONOutput(out, result); err != nil {
			return err
		}
	}
	if hookErr != nil {
		if result.Error != "" {
			return fmt.Errorf(result.Error)
		}
		return hookErr
	}

	if flags.OutputJSON {
		return nil
	}

	switch result.Status {
	case core.AgentCompletionStatusCreated:
		cellID := ""
		if result.CellID != nil {
			cellID = *result.CellID
		}
		fmt.Fprintf(out, "created run=%s cell=%s branch=%s source=%s\n", result.RunID, cellID, result.Branch, result.Source)
	case core.AgentCompletionStatusNoChange:
		fmt.Fprintf(out, "no_change run=%s branch=%s source=%s\n", result.RunID, result.Branch, result.Source)
	case core.AgentCompletionStatusDuplicate:
		cellID := ""
		if result.CellID != nil {
			cellID = *result.CellID
		}
		if cellID == "" {
			fmt.Fprintf(out, "duplicate run=%s source=%s\n", result.RunID, result.Source)
		} else {
			fmt.Fprintf(out, "duplicate run=%s cell=%s branch=%s source=%s\n", result.RunID, cellID, result.Branch, result.Source)
		}
	default:
		return fmt.Errorf("unexpected hook result status: %s", result.Status)
	}
	return nil
}

func validateHookCompleteFlags(flags hookCompleteFlags) error {
	if strings.TrimSpace(flags.RunID) == "" {
		return fmt.Errorf("missing required flag: --run-id")
	}
	if strings.TrimSpace(flags.Agent) == "" {
		return fmt.Errorf("missing required flag: --agent")
	}
	if strings.TrimSpace(flags.Message) == "" {
		return fmt.Errorf("missing required flag: --message")
	}
	return nil
}
