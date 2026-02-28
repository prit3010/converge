package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRestorePreservesUntrackedAndRemovesTrackedMissing(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	mainPath := filepath.Join(svc.ProjectDir, "main.go")
	helperPath := filepath.Join(svc.ProjectDir, "helper.go")
	untrackedPath := filepath.Join(svc.ProjectDir, "notes.tmp")

	if err := os.WriteFile(mainPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main v1: %v", err)
	}
	c1, err := svc.CreateCell(ctx, SnapOptions{Message: "v1", RunEval: false})
	if err != nil {
		t.Fatalf("create c1: %v", err)
	}

	if err := os.WriteFile(mainPath, []byte("package main\nfunc main(){}\n"), 0o644); err != nil {
		t.Fatalf("write main v2: %v", err)
	}
	if err := os.WriteFile(helperPath, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	if _, err := svc.CreateCell(ctx, SnapOptions{Message: "v2", RunEval: false}); err != nil {
		t.Fatalf("create c2: %v", err)
	}

	if err := os.WriteFile(untrackedPath, []byte("keep me"), 0o644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}

	safety, err := svc.RestoreCell(ctx, c1.ID)
	if err != nil {
		t.Fatalf("restore cell: %v", err)
	}
	if safety.Source != "restore_safety" {
		t.Fatalf("expected restore_safety source, got %s", safety.Source)
	}

	if _, err := os.Stat(helperPath); !os.IsNotExist(err) {
		t.Fatalf("expected helper.go removed, err=%v", err)
	}
	if _, err := os.Stat(untrackedPath); err != nil {
		t.Fatalf("expected untracked file preserved, err=%v", err)
	}
}

func TestRestoreResetsBranchHeadForNextSnapParent(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	mainPath := filepath.Join(svc.ProjectDir, "main.go")
	if err := os.WriteFile(mainPath, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write baseline: %v", err)
	}
	c1, err := svc.CreateCell(ctx, SnapOptions{Message: "baseline", RunEval: false})
	if err != nil {
		t.Fatalf("create c1: %v", err)
	}

	if err := os.WriteFile(mainPath, []byte("package main\nfunc main() { println(\"v2\") }\n"), 0o644); err != nil {
		t.Fatalf("write v2: %v", err)
	}
	if _, err := svc.CreateCell(ctx, SnapOptions{Message: "v2", RunEval: false}); err != nil {
		t.Fatalf("create c2: %v", err)
	}

	if _, err := svc.RestoreCell(ctx, c1.ID); err != nil {
		t.Fatalf("restore to c1: %v", err)
	}

	if err := os.WriteFile(mainPath, []byte("package main\nfunc main() { println(\"after-restore\") }\n"), 0o644); err != nil {
		t.Fatalf("write after restore: %v", err)
	}
	cAfter, err := svc.CreateCell(ctx, SnapOptions{Message: "after restore", RunEval: false})
	if err != nil {
		t.Fatalf("create post-restore cell: %v", err)
	}
	if cAfter.ParentID == nil || *cAfter.ParentID != c1.ID {
		t.Fatalf("expected post-restore parent %s, got %v", c1.ID, cAfter.ParentID)
	}
}
