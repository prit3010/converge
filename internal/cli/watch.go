package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prittamravi/converge/internal/config"
	"github.com/prittamravi/converge/internal/core"
	"github.com/prittamravi/converge/internal/watch"
	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	var debounce time.Duration
	cmd := &cobra.Command{
		Use:   "watch",
		Short: "Watch file changes and auto-capture cells",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runWatch(cwd, debounce)
		},
	}
	cmd.Flags().DurationVar(&debounce, "debounce", config.DefaultWatchDebounce, "Debounce window before auto-snapshot")
	return cmd
}

func runWatch(projectDir string, debounce time.Duration) error {
	svc, err := openService(projectDir)
	if err != nil {
		return err
	}
	defer svc.DB.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fmt.Printf("Watching %s (debounce %s). Press Ctrl+C to stop.\n", projectDir, debounce)
	return watch.Watch(ctx, projectDir, debounce, func() error {
		if svc.IsRestoreInProgress() {
			return nil
		}
		cell, created, err := svc.CreateCellIfChanged(context.Background(), core.SnapOptions{
			Message: "auto-captured by watch",
			Source:  "watch",
			RunEval: false,
		})
		if err != nil {
			return err
		}
		if created {
			fmt.Printf("[watch] %s branch=%s files=%d loc=%d delta=%+d\n", cell.ID, cell.Branch, cell.TotalFiles, cell.TotalLOC, cell.LOCDelta)
		}
		return nil
	})
}
