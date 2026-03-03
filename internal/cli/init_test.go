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

	ignorePath := filepath.Join(projectDir, config.IgnoreFileName)
	data, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("expected %s to be created: %v", config.IgnoreFileName, err)
	}
	if len(data) == 0 {
		t.Fatalf("expected %s content to be non-empty", config.IgnoreFileName)
	}
}

func TestRunInitPreservesExistingConvergeIgnore(t *testing.T) {
	projectDir := t.TempDir()
	ignorePath := filepath.Join(projectDir, config.IgnoreFileName)
	original := []byte("# custom\nsecret.txt\n")
	if err := os.WriteFile(ignorePath, original, 0o644); err != nil {
		t.Fatalf("seed %s: %v", config.IgnoreFileName, err)
	}

	if err := runInit(projectDir); err != nil {
		t.Fatalf("run init: %v", err)
	}

	data, err := os.ReadFile(ignorePath)
	if err != nil {
		t.Fatalf("read %s: %v", config.IgnoreFileName, err)
	}
	if string(data) != string(original) {
		t.Fatalf("expected existing %s content to be preserved", config.IgnoreFileName)
	}
}
