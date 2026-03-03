package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prit3010/converge/internal/diff"
	"github.com/prit3010/converge/internal/snapshot"
	"github.com/spf13/cobra"
)

type diffFileJSON struct {
	Path   string `json:"path"`
	Status string `json:"status"`
	Patch  string `json:"patch,omitempty"`
}

func newDiffCmd() *cobra.Command {
	var noColor bool
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "diff <cellA> <cellB>",
		Short: "Show differences between two cells",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runDiff(cwd, args[0], args[1], noColor, outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable ANSI colors in diff output")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runDiff(projectDir, cellA, cellB string, noColor bool, outputJSON bool, out io.Writer) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	if _, err := svc.DB.GetCell(cellA); err != nil {
		return notFoundErrorf("cell %s not found", cellA)
	}
	if _, err := svc.DB.GetCell(cellB); err != nil {
		return notFoundErrorf("cell %s not found", cellB)
	}

	manifestAEntries, err := svc.DB.GetManifest(cellA)
	if err != nil {
		return err
	}
	manifestBEntries, err := svc.DB.GetManifest(cellB)
	if err != nil {
		return err
	}
	mapA := make(map[string]string, len(manifestAEntries))
	for _, entry := range manifestAEntries {
		mapA[entry.Path] = entry.Hash
	}
	mapB := make(map[string]string, len(manifestBEntries))
	for _, entry := range manifestBEntries {
		mapB[entry.Path] = entry.Hash
	}

	result := diff.CompareManifests(mapA, mapB)
	files := make([]diffFileJSON, 0, len(result.Added)+len(result.Modified)+len(result.Removed))
	for _, path := range result.Added {
		files = append(files, diffFileJSON{Path: path, Status: "added"})
	}
	for _, path := range result.Removed {
		files = append(files, diffFileJSON{Path: path, Status: "removed"})
	}
	for _, path := range result.Modified {
		file := diffFileJSON{Path: path, Status: "modified"}
		oldData, errOld := svc.Store.Read(mapA[path])
		newData, errNew := svc.Store.Read(mapB[path])
		if errOld == nil && errNew == nil && snapshot.IsText(oldData) && snapshot.IsText(newData) {
			file.Patch = diff.UnifiedDiff(path, string(oldData), string(newData))
		}
		files = append(files, file)
	}

	totalChanged := len(result.Added) + len(result.Modified) + len(result.Removed)
	if outputJSON {
		return writeCommandSuccessJSON(out, "diff", map[string]any{
			"cell_a": cellA,
			"cell_b": cellB,
			"summary": map[string]int{
				"added":         len(result.Added),
				"modified":      len(result.Modified),
				"removed":       len(result.Removed),
				"total_changed": totalChanged,
			},
			"files": files,
		})
	}

	palette := newDiffPalette(noColor)
	fmt.Fprintf(out, "%s %s %s %s\n", palette.bold("Diff"), palette.bold(cellA), palette.dim("->"), palette.bold(cellB))
	fmt.Fprintf(
		out,
		"%s %s %s %s %s %s",
		palette.dim("Summary:"),
		palette.green(fmt.Sprintf("+%d added", len(result.Added))),
		palette.yellow(fmt.Sprintf("~%d modified", len(result.Modified))),
		palette.red(fmt.Sprintf("-%d removed", len(result.Removed))),
		palette.dim("|"),
		fmt.Sprintf("%d total changed", totalChanged),
	)
	fmt.Fprintln(out)
	fmt.Fprintln(out)

	if len(result.Added) > 0 {
		fmt.Fprintf(out, "%s (%d):\n", palette.green("Added"), len(result.Added))
		for _, path := range result.Added {
			fmt.Fprintf(out, "  %s %s\n", palette.green("+"), path)
		}
		fmt.Fprintln(out)
	}
	if len(result.Removed) > 0 {
		fmt.Fprintf(out, "%s (%d):\n", palette.red("Removed"), len(result.Removed))
		for _, path := range result.Removed {
			fmt.Fprintf(out, "  %s %s\n", palette.red("-"), path)
		}
		fmt.Fprintln(out)
	}
	if len(result.Modified) > 0 {
		fmt.Fprintf(out, "%s (%d):\n", palette.yellow("Modified"), len(result.Modified))
		for _, path := range result.Modified {
			fmt.Fprintf(out, "  %s %s\n", palette.yellow("~"), path)
		}
		fmt.Fprintln(out)

		for _, path := range result.Modified {
			oldData, err := svc.Store.Read(mapA[path])
			if err != nil {
				continue
			}
			newData, err := svc.Store.Read(mapB[path])
			if err != nil {
				continue
			}
			if !snapshot.IsText(oldData) || !snapshot.IsText(newData) {
				fmt.Fprintf(out, "%s %s\n", palette.dim("binary diff skipped for"), path)
				continue
			}
			unified := diff.UnifiedDiff(path, string(oldData), string(newData))
			if unified != "" {
				fmt.Fprintf(out, "%s %s\n", palette.cyan("Patch:"), palette.bold(path))
				fmt.Fprintln(out, colorizeUnifiedDiff(unified, palette))
				fmt.Fprintln(out)
			}
		}
	}
	if len(result.Added) == 0 && len(result.Modified) == 0 && len(result.Removed) == 0 {
		fmt.Fprintln(out, palette.green("No differences."))
	}
	return nil
}

func colorizeUnifiedDiff(unified string, palette diffPalette) string {
	lines := strings.Split(unified, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
			out = append(out, palette.cyan(line))
		case strings.HasPrefix(line, "@@"):
			out = append(out, palette.yellow(line))
		case strings.HasPrefix(line, "+"):
			out = append(out, palette.green(line))
		case strings.HasPrefix(line, "-"):
			out = append(out, palette.red(line))
		default:
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

type diffPalette struct {
	enabled bool
}

func newDiffPalette(noColor bool) diffPalette {
	if noColor {
		return diffPalette{enabled: false}
	}
	if os.Getenv("NO_COLOR") != "" {
		return diffPalette{enabled: false}
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return diffPalette{enabled: false}
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return diffPalette{enabled: false}
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return diffPalette{enabled: false}
	}
	return diffPalette{enabled: true}
}

func (p diffPalette) wrap(code string, text string) string {
	if !p.enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (p diffPalette) dim(text string) string {
	return p.wrap("2", text)
}

func (p diffPalette) bold(text string) string {
	return p.wrap("1", text)
}

func (p diffPalette) red(text string) string {
	return p.wrap("31", text)
}

func (p diffPalette) green(text string) string {
	return p.wrap("32", text)
}

func (p diffPalette) yellow(text string) string {
	return p.wrap("33", text)
}

func (p diffPalette) cyan(text string) string {
	return p.wrap("36", text)
}
