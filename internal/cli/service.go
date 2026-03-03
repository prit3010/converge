package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/prit3010/converge/internal/config"
	"github.com/prit3010/converge/internal/core"
	"github.com/prit3010/converge/internal/db"
	"github.com/prit3010/converge/internal/eval"
	"github.com/prit3010/converge/internal/store"
)

func openService(projectDir string) (*core.Service, error) {
	stateDir := filepath.Join(projectDir, config.StateDirName)
	if st, err := os.Stat(stateDir); err != nil || !st.IsDir() {
		return nil, fmt.Errorf("not a converge repository (run 'converge init' first)")
	}

	policy, err := config.LoadRepoPolicy(projectDir)
	if err != nil {
		return nil, fmt.Errorf("load repository policy: %w", err)
	}
	database, err := db.Open(filepath.Join(stateDir, config.DBFileName))
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}
	objectStore := store.New(filepath.Join(stateDir, config.ObjectsDirName))
	svc := core.NewService(projectDir, database, objectStore, eval.NewRunner())
	svc.SetPolicy(policy)
	return svc, nil
}
