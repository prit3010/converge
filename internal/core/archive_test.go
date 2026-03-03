package core

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prit3010/converge/internal/db"
)

func TestRotateOnGitCommitArchivesAndCreatesTrackedBaseline(t *testing.T) {
	requireGit(t)
	svc := newTestService(t)
	ctx := context.Background()

	initGitRepo(t, svc.ProjectDir)

	mainPath := filepath.Join(svc.ProjectDir, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	sha := gitCommitAll(t, svc.ProjectDir, "initial")
	branch := gitOutput(t, svc.ProjectDir, "branch", "--show-current")
	committedAt := gitOutput(t, svc.ProjectDir, "log", "-1", "--format=%cI", sha)

	if _, err := svc.CreateCell(ctx, SnapOptions{Message: "pre-archive", RunEval: false}); err != nil {
		t.Fatalf("create pre-archive cell: %v", err)
	}

	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "notes.tmp"), []byte("scratch"), 0o644); err != nil {
		t.Fatalf("write untracked note: %v", err)
	}

	result, err := svc.RotateOnGitCommit(ctx, GitCommitMetadata{
		SHA:         sha,
		Branch:      branch,
		Subject:     "initial",
		CommittedAt: committedAt,
	})
	if err != nil {
		t.Fatalf("rotate on git commit: %v", err)
	}
	if result.Archive == nil {
		t.Fatalf("expected archive metadata")
	}
	if result.Archive.CellCount != 1 {
		t.Fatalf("expected archived cell count 1, got %d", result.Archive.CellCount)
	}
	if result.BaselineCell == nil {
		t.Fatalf("expected baseline cell")
	}
	if result.BaselineCell.Source != gitCommitBaselineSource {
		t.Fatalf("expected baseline source %s, got %s", gitCommitBaselineSource, result.BaselineCell.Source)
	}

	activeCount, err := svc.DB.CountCells()
	if err != nil {
		t.Fatalf("count active cells: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected exactly 1 fresh baseline cell, got %d", activeCount)
	}

	manifest, err := svc.DB.GetManifest(result.BaselineCell.ID)
	if err != nil {
		t.Fatalf("load baseline manifest: %v", err)
	}
	paths := make(map[string]struct{}, len(manifest))
	for _, entry := range manifest {
		paths[entry.Path] = struct{}{}
	}
	if _, ok := paths["main.go"]; !ok {
		t.Fatalf("expected tracked file main.go in baseline manifest")
	}
	if _, ok := paths["notes.tmp"]; ok {
		t.Fatalf("did not expect untracked notes.tmp in baseline manifest")
	}

	archiveDBPath, _, err := svc.ArchiveStatePaths(result.Archive.ArchiveID)
	if err != nil {
		t.Fatalf("resolve archive state path: %v", err)
	}
	archiveDB, err := db.Open(archiveDBPath)
	if err != nil {
		t.Fatalf("open archive db: %v", err)
	}
	defer archiveDB.Close()

	archivedCount, err := archiveDB.CountCells()
	if err != nil {
		t.Fatalf("count archived cells: %v", err)
	}
	if archivedCount != 1 {
		t.Fatalf("expected 1 archived cell, got %d", archivedCount)
	}
}

func TestRotateOnGitCommitSkipsEmptyArchiveAndCreatesBaseline(t *testing.T) {
	requireGit(t)
	svc := newTestService(t)
	ctx := context.Background()

	initGitRepo(t, svc.ProjectDir)
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	sha := gitCommitAll(t, svc.ProjectDir, "initial")
	branch := gitOutput(t, svc.ProjectDir, "branch", "--show-current")

	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "tmp.log"), []byte("untracked"), 0o644); err != nil {
		t.Fatalf("write untracked tmp.log: %v", err)
	}

	result, err := svc.RotateOnGitCommit(ctx, GitCommitMetadata{
		SHA:     sha,
		Branch:  branch,
		Subject: "initial",
	})
	if err != nil {
		t.Fatalf("rotate on empty state: %v", err)
	}
	if result.Archive != nil {
		t.Fatalf("expected no archive metadata when active state is empty")
	}

	archives, err := svc.ListArchiveMetadata()
	if err != nil {
		t.Fatalf("list archives: %v", err)
	}
	if len(archives) != 0 {
		t.Fatalf("expected no archived entries, got %d", len(archives))
	}

	if result.BaselineCell == nil {
		t.Fatalf("expected baseline cell")
	}
	manifest, err := svc.DB.GetManifest(result.BaselineCell.ID)
	if err != nil {
		t.Fatalf("load baseline manifest: %v", err)
	}
	paths := make(map[string]struct{}, len(manifest))
	for _, entry := range manifest {
		paths[entry.Path] = struct{}{}
	}
	if _, ok := paths["main.go"]; !ok {
		t.Fatalf("expected tracked main.go in baseline")
	}
	if _, ok := paths["tmp.log"]; ok {
		t.Fatalf("did not expect untracked tmp.log in baseline")
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for archive rotation tests")
	}
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "converge-tests@example.com")
	runGit(t, dir, "config", "user.name", "Converge Tests")
}

func gitCommitAll(t *testing.T, dir string, subject string) string {
	t.Helper()
	runGit(t, dir, "add", "-A")
	runGit(t, dir, "commit", "-m", subject)
	return gitOutput(t, dir, "rev-parse", "HEAD")
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = gitOutput(t, dir, args...)
}
