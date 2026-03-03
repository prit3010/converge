package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

var (
	// Build-time metadata set via -ldflags during release builds.
	Version   = "dev"
	Commit    = "none"
	BuildDate = "unknown"
)

func newVersionCmd() *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Show converge version metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runVersion(outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runVersion(outputJSON bool, out io.Writer) error {
	payload := map[string]any{
		"version":    Version,
		"commit":     Commit,
		"build_date": BuildDate,
	}
	if outputJSON {
		return writeCommandSuccessJSON(out, "version", payload)
	}

	fmt.Fprintf(out, "converge %s\n", Version)
	fmt.Fprintf(out, "  commit: %s\n", Commit)
	fmt.Fprintf(out, "  build date: %s\n", BuildDate)
	return nil
}
