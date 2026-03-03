package snapshot

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/prittamravi/converge/internal/config"
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

func TestCaptureHonorsIgnorePolicy(t *testing.T) {
	project := t.TempDir()
	objects := t.TempDir()

	if err := os.WriteFile(filepath.Join(project, ".env"), []byte("SECRET=1\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, config.IgnoreFileName), []byte(".env\n"), 0o644); err != nil {
		t.Fatalf("write .convergeignore: %v", err)
	}

	policy, err := config.LoadRepoPolicy(project)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}
	s := NewWithPolicy(store.New(objects), policy)
	manifest, err := s.Capture(project)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if _, ok := manifest[".env"]; ok {
		t.Fatalf("expected .env to be ignored")
	}
}

func TestCaptureSkipsBinaryByDefault(t *testing.T) {
	project := t.TempDir()
	objects := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "bin.dat"), []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	s := New(store.New(objects))
	manifest, err := s.Capture(project)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(manifest) != 0 {
		t.Fatalf("expected binary file to be skipped, got %d entries", len(manifest))
	}
}

func TestCaptureFailsOnBinaryWhenConfigured(t *testing.T) {
	project := t.TempDir()
	objects := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "bin.dat"), []byte{0x00, 0x01, 0x02}, 0o644); err != nil {
		t.Fatalf("write binary: %v", err)
	}

	policy := config.DefaultPolicy()
	policy.Snapshot.BinaryPolicy = config.BinaryPolicyFail
	s := NewWithPolicy(store.New(objects), policy)
	if _, err := s.Capture(project); err == nil {
		t.Fatalf("expected binary policy fail error")
	}
}

func TestCaptureSkipsLargeFilesByPolicy(t *testing.T) {
	project := t.TempDir()
	objects := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, "large.txt"), []byte("1234567890"), 0o644); err != nil {
		t.Fatalf("write large file: %v", err)
	}

	policy := config.DefaultPolicy()
	policy.Snapshot.MaxFileSizeBytes = 4
	s := NewWithPolicy(store.New(objects), policy)
	manifest, err := s.Capture(project)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if len(manifest) != 0 {
		t.Fatalf("expected oversized file to be skipped")
	}
	skipped := s.LastSkipped()
	if len(skipped) != 1 || skipped[0].Reason != "max_file_size_exceeded" {
		t.Fatalf("unexpected skipped reasons: %+v", skipped)
	}
}
