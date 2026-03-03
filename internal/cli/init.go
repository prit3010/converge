package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/prit3010/converge/internal/config"
	"github.com/prit3010/converge/internal/db"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize a converge repository in the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runInit(cwd)
		},
	}
}

func runInit(projectDir string) error {
	stateDir := filepath.Join(projectDir, config.StateDirName)
	objectsDir := filepath.Join(stateDir, config.ObjectsDirName)
	if err := os.MkdirAll(objectsDir, 0o755); err != nil {
		return fmt.Errorf("create state dirs: %w", err)
	}
	database, err := db.Open(filepath.Join(stateDir, config.DBFileName))
	if err != nil {
		return fmt.Errorf("initialize database: %w", err)
	}
	defer database.Close()

	ignorePath := filepath.Join(projectDir, config.IgnoreFileName)
	if _, err := os.Stat(ignorePath); os.IsNotExist(err) {
		if err := os.WriteFile(ignorePath, []byte(config.DefaultConvergeIgnoreTemplate), 0o644); err != nil {
			return fmt.Errorf("create %s: %w", config.IgnoreFileName, err)
		}
	}

	fmt.Printf("Initialized converge in %s\n", stateDir)
	return nil
}
