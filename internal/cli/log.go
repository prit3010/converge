package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/prittamravi/converge/internal/db"
	"github.com/spf13/cobra"
)

func newLogCmd() *cobra.Command {
	var limit int
	var noColor bool
	var branch string
	var showAll bool
	var outputJSON bool
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Show cell history",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runLog(cwd, limit, noColor, branch, showAll, outputJSON, cmd.OutOrStdout())
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 20, "Maximum number of cells to print")
	cmd.Flags().BoolVar(&noColor, "no-color", false, "Disable ANSI colors in log output")
	cmd.Flags().StringVar(&branch, "branch", "", "Show history for a specific branch")
	cmd.Flags().BoolVar(&showAll, "all", false, "Show history across all branches")
	cmd.Flags().BoolVar(&outputJSON, "json", false, "Print machine-readable JSON output")
	return cmd
}

func runLog(projectDir string, limit int, noColor bool, branch string, showAll bool, outputJSON bool, out io.Writer) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	if showAll && strings.TrimSpace(branch) != "" {
		return validationErrorf("cannot use --branch and --all together")
	}

	activeBranch, err := svc.ActiveBranch()
	if err != nil {
		return err
	}

	headCellID := ""
	if v, err := svc.DB.GetMeta("head_cell"); err == nil {
		headCellID = strings.TrimSpace(v)
	}

	targetBranch := strings.TrimSpace(branch)
	if targetBranch == "" && !showAll {
		targetBranch = activeBranch
	}

	var cells []db.Cell
	if showAll {
		cells, err = svc.DB.ListCells(limit)
	} else {
		cells, err = svc.DB.ListCellsByBranch(targetBranch, limit)
	}
	if err != nil {
		return err
	}
	if outputJSON {
		return writeCommandSuccessJSON(out, "log", map[string]any{
			"active_branch": activeBranch,
			"head_cell_id":  headCellID,
			"scope": map[string]any{
				"all":    showAll,
				"branch": targetBranch,
				"limit":  limit,
			},
			"cells": cells,
		})
	}

	if len(cells) == 0 {
		if showAll {
			fmt.Fprintln(out, "No cells yet. Run 'converge snap -m \"message\"' to create one.")
		} else {
			fmt.Fprintf(out, "No cells on branch %q yet. Run 'converge snap -m \"message\"' to create one.\n", targetBranch)
		}
		return nil
	}

	palette := newLogPalette(noColor)
	if showAll {
		fmt.Fprintf(out, "Showing %d most recent cells across all branches (active: %s)\n\n", len(cells), activeBranch)
	} else {
		fmt.Fprintf(out, "Showing %d most recent cells on branch %s\n\n", len(cells), targetBranch)
	}
	for i, cell := range cells {
		if i > 0 {
			fmt.Fprintln(out)
		}
		printCell(out, cell, cell.ID == headCellID, palette)
	}
	return nil
}

func printCell(out io.Writer, cell db.Cell, isHead bool, palette logPalette) {
	headLabel := ""
	if isHead {
		headLabel = "  " + palette.green("HEAD")
	}
	branchLabel := ""
	if strings.TrimSpace(cell.Branch) != "" {
		branchLabel = "  " + palette.cyan("["+cell.Branch+"]")
	}
	fmt.Fprintf(out, "[%s]%s%s\n", palette.bold(cell.ID), headLabel, branchLabel)
	fmt.Fprintf(out, "  %s : %s\n", palette.dim("time"), cell.Timestamp)
	fmt.Fprintf(out, "  %s : %q\n", palette.dim("message"), cell.Message)

	fmt.Fprintf(out, "  %s : source=%s", palette.dim("metadata"), palette.cyan(cell.Source))
	if cell.Agent != nil {
		fmt.Fprintf(out, " | agent=%s", palette.cyan(*cell.Agent))
	}
	if cell.Tags != nil {
		fmt.Fprintf(out, " | tags=%s", palette.cyan(*cell.Tags))
	}
	fmt.Fprintln(out)

	fmt.Fprintf(out, "  %s : files %s %s %s | lines %s %s\n",
		palette.dim("changes"),
		palette.green(fmt.Sprintf("+%d", cell.FilesAdded)),
		palette.yellow(fmt.Sprintf("~%d", cell.FilesModified)),
		palette.red(fmt.Sprintf("-%d", cell.FilesRemoved)),
		palette.green(fmt.Sprintf("+%d", cell.LinesAdded)),
		palette.red(fmt.Sprintf("-%d", cell.LinesRemoved)),
	)
	locDelta := fmt.Sprintf("%+d", cell.LOCDelta)
	if cell.LOCDelta > 0 {
		locDelta = palette.red(locDelta)
	} else if cell.LOCDelta < 0 {
		locDelta = palette.green(locDelta)
	} else {
		locDelta = palette.yellow(locDelta)
	}
	fmt.Fprintf(out, "  complexity(LOC): total %d (delta %s) across %d files\n", cell.TotalLOC, locDelta, cell.TotalFiles)

	switch {
	case cell.EvalRequested && !cell.EvalRan:
		fmt.Fprintf(out, "  %s : %s\n", palette.dim("eval"), palette.yellow("pending"))
	case cell.EvalRan:
		parts := make([]string, 0, 4)
		if cell.TestsPassed != nil || cell.TestsFailed != nil {
			passed := 0
			failed := 0
			if cell.TestsPassed != nil {
				passed = *cell.TestsPassed
			}
			if cell.TestsFailed != nil {
				failed = *cell.TestsFailed
			}
			testsLabel := fmt.Sprintf("tests %d/%d", passed, passed+failed)
			if failed > 0 {
				testsLabel = palette.red(testsLabel)
			} else {
				testsLabel = palette.green(testsLabel)
			}
			parts = append(parts, testsLabel)
		}
		if cell.LintErrors != nil {
			lintLabel := fmt.Sprintf("lint %d", *cell.LintErrors)
			if *cell.LintErrors > 0 {
				lintLabel = palette.red(lintLabel)
			} else {
				lintLabel = palette.green(lintLabel)
			}
			parts = append(parts, lintLabel)
		}
		if cell.TypeErrors != nil {
			typesLabel := fmt.Sprintf("types %d", *cell.TypeErrors)
			if *cell.TypeErrors > 0 {
				typesLabel = palette.red(typesLabel)
			} else {
				typesLabel = palette.green(typesLabel)
			}
			parts = append(parts, typesLabel)
		}
		if cell.EvalSkipped != nil {
			parts = append(parts, palette.yellow(fmt.Sprintf("skipped %s", *cell.EvalSkipped)))
		}
		if cell.EvalError != nil {
			parts = append(parts, palette.red(fmt.Sprintf("error %s", *cell.EvalError)))
		}
		if len(parts) == 0 {
			fmt.Fprintf(out, "  %s : %s\n", palette.dim("eval"), palette.green("complete"))
		} else {
			fmt.Fprintf(out, "  %s : %s\n", palette.dim("eval"), strings.Join(parts, " | "))
		}
	default:
		fmt.Fprintf(out, "  %s : %s\n", palette.dim("eval"), "not requested")
	}
}

type logPalette struct {
	enabled bool
}

func newLogPalette(noColor bool) logPalette {
	if noColor {
		return logPalette{enabled: false}
	}
	if os.Getenv("NO_COLOR") != "" {
		return logPalette{enabled: false}
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return logPalette{enabled: false}
	}
	info, err := os.Stdout.Stat()
	if err != nil {
		return logPalette{enabled: false}
	}
	if info.Mode()&os.ModeCharDevice == 0 {
		return logPalette{enabled: false}
	}
	return logPalette{enabled: true}
}

func (p logPalette) wrap(code string, text string) string {
	if !p.enabled {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (p logPalette) dim(text string) string {
	return p.wrap("2", text)
}

func (p logPalette) bold(text string) string {
	return p.wrap("1", text)
}

func (p logPalette) red(text string) string {
	return p.wrap("31", text)
}

func (p logPalette) green(text string) string {
	return p.wrap("32", text)
}

func (p logPalette) yellow(text string) string {
	return p.wrap("33", text)
}

func (p logPalette) cyan(text string) string {
	return p.wrap("36", text)
}
