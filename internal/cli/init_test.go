package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prittamravi/converge/internal/config"
)

func TestRunInitIsIdempotent(t *testing.T) {
	projectDir := t.TempDir()
	if err := runInit(projectDir); err != nil {
		t.Fatalf("first init: %v", err)
	}
	if err := runInit(projectDir); err != nil {
		t.Fatalf("second init: %v", err)
	}

	stateDir := filepath.Join(projectDir, config.StateDirName)
	if st, err := os.Stat(stateDir); err != nil || !st.IsDir() {
		t.Fatalf("expected state dir %s", stateDir)
	}
}
