package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/prittamravi/converge/internal/config"
	"github.com/prittamravi/converge/internal/core"
	"github.com/prittamravi/converge/internal/db"
	"github.com/prittamravi/converge/internal/eval"
	"github.com/prittamravi/converge/internal/store"
)

func openService(projectDir string) (*core.Service, error) {
	stateDir := filepath.Join(projectDir, config.StateDirName)
	if st, err := os.Stat(stateDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("not a converge repository (run 'converge init' first)")
	}
	database, err := db.Open(filepath.Join(stateDir, config.DBFileName))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	objectStore := store.New(filepath.Join(stateDir, config.ObjectsDirName))
	return core.NewService(projectDir, database, objectStore, eval.NewRunner()), nil
}
