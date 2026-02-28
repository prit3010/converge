package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prittamravi/converge/internal/store"
)

func TestCaptureSkipsIgnoredDirs(t *testing.T) {
	project := t.TempDir()
	objects := t.TempDir()

	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(project, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, ".git", "config"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write .git config: %v", err)
	}

	s := New(store.New(objects))
	manifest, err := s.Capture(project)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(manifest) != 1 {
		t.Fatalf("expected 1 tracked file, got %d", len(manifest))
	}
	if _, ok := manifest["main.go"]; !ok {
		t.Fatalf("expected main.go in manifest")
	}
}

func TestCaptureDedupeHashes(t *testing.T) {
	project := t.TempDir()
	objects := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "a.txt"), []byte("same"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "b.txt"), []byte("same"), 0o644); err != nil {
		t.Fatalf("write b.txt: %v", err)
	}

	s := New(store.New(objects))
	manifest, err := s.Capture(project)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if manifest["a.txt"].Hash != manifest["b.txt"].Hash {
		t.Fatalf("expected identical content hashes")
	}
}
