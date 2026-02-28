package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/prittamravi/converge/internal/db"
	"github.com/prittamravi/converge/internal/eval"
	"github.com/prittamravi/converge/internal/store"
)

func TestCreateCellSequenceAndParent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	c1, err := svc.CreateCell(ctx, SnapOptions{Message: "first", Source: "manual", RunEval: false})
	if err != nil {
		t.Fatalf("create first cell: %v", err)
	}
	if c1.ID != "c_000001" {
		t.Fatalf("expected c_000001, got %s", c1.ID)
	}
	if c1.ParentID != nil {
		t.Fatalf("expected nil parent for first cell")
	}

	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("rewrite main.go: %v", err)
	}
	c2, err := svc.CreateCell(ctx, SnapOptions{Message: "second", Source: "manual", RunEval: false})
	if err != nil {
		t.Fatalf("create second cell: %v", err)
	}
	if c2.ID != "c_000002" {
		t.Fatalf("expected c_000002, got %s", c2.ID)
	}
	if c2.ParentID == nil || *c2.ParentID != c1.ID {
		t.Fatalf("expected parent %s, got %v", c1.ID, c2.ParentID)
	}
}

func TestCreateCellIfChangedNoop(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "a.txt"), []byte("same"), 0o644); err != nil {
		t.Fatalf("write a.txt: %v", err)
	}
	if _, err := svc.CreateCell(ctx, SnapOptions{Message: "initial", RunEval: false}); err != nil {
		t.Fatalf("create initial cell: %v", err)
	}
	cell, created, err := svc.CreateCellIfChanged(ctx, SnapOptions{Message: "watch", Source: "watch", RunEval: false})
	if err != nil {
		t.Fatalf("create if changed: %v", err)
	}
	if created {
		t.Fatalf("expected no cell creation, got %+v", cell)
	}
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	projectDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectDir, ".converge", "objects"), 0o755); err != nil {
		t.Fatalf("mkdir state dirs: %v", err)
	}
	database, err := db.Open(filepath.Join(projectDir, ".converge", "converge.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})
	objectStore := store.New(filepath.Join(projectDir, ".converge", "objects"))
	return NewService(projectDir, database, objectStore, eval.NewRunner())
}

func TestEvaluateCellPersistsResult(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	cell, err := svc.CreateCell(ctx, SnapOptions{Message: "base", RunEval: false})
	if err != nil {
		t.Fatalf("create cell: %v", err)
	}

	if _, err := svc.EvaluateCell(ctx, cell.ID); err != nil {
		t.Fatalf("evaluate cell: %v", err)
	}
	updated, err := svc.DB.GetCell(cell.ID)
	if err != nil {
		t.Fatalf("get updated cell: %v", err)
	}
	if !updated.EvalRan {
		t.Fatalf("expected eval ran true")
	}
}

func TestBranchForkAndSwitchUsesBranchHeadParent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	mainPath := filepath.Join(svc.ProjectDir, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	mainBase, err := svc.CreateCell(ctx, SnapOptions{Message: "main base", RunEval: false})
	if err != nil {
		t.Fatalf("create main base cell: %v", err)
	}
	if mainBase.Branch != "main" {
		t.Fatalf("expected main branch, got %s", mainBase.Branch)
	}

	if _, err := svc.ForkBranch("feature-a", true); err != nil {
		t.Fatalf("fork feature-a: %v", err)
	}
	active, err := svc.ActiveBranch()
	if err != nil {
		t.Fatalf("active branch: %v", err)
	}
	if active != "feature-a" {
		t.Fatalf("expected feature-a active, got %s", active)
	}

	if err := os.WriteFile(mainPath, []byte("package main\nfunc feature() {}\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	featureCell, err := svc.CreateCell(ctx, SnapOptions{Message: "feature work", RunEval: false})
	if err != nil {
		t.Fatalf("create feature cell: %v", err)
	}
	if featureCell.Branch != "feature-a" {
		t.Fatalf("expected feature-a branch, got %s", featureCell.Branch)
	}

	safety, switchedTo, err := svc.SwitchBranch(ctx, "main")
	if err != nil {
		t.Fatalf("switch to main: %v", err)
	}
	if safety == nil {
		t.Fatalf("expected safety snapshot on switch")
	}
	if switchedTo.ID != mainBase.ID {
		t.Fatalf("expected switched head %s, got %s", mainBase.ID, switchedTo.ID)
	}

	if err := os.WriteFile(mainPath, []byte("package main\nfunc mainline() {}\n"), 0o644); err != nil {
		t.Fatalf("write mainline file: %v", err)
	}
	mainFollowup, err := svc.CreateCell(ctx, SnapOptions{Message: "main followup", RunEval: false})
	if err != nil {
		t.Fatalf("create main followup: %v", err)
	}
	if mainFollowup.Branch != "main" {
		t.Fatalf("expected main branch, got %s", mainFollowup.Branch)
	}
	if mainFollowup.ParentID == nil || *mainFollowup.ParentID != mainBase.ID {
		t.Fatalf("expected main followup parent %s, got %v", mainBase.ID, mainFollowup.ParentID)
	}

	featureHead, err := svc.DB.LatestCellByBranch("feature-a")
	if err != nil {
		t.Fatalf("latest feature head: %v", err)
	}
	if featureHead == nil || featureHead.ID != safety.ID {
		t.Fatalf("expected feature head advanced to switch safety cell %s, got %+v", safety.ID, featureHead)
	}
}
