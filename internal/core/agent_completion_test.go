package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleAgentCompletionCreated(t *testing.T) {
	svc := newTestService(t)
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	result, err := svc.HandleAgentCompletion(context.Background(), AgentCompletionOptions{
		RunID:   "run-created",
		Agent:   "codex",
		Message: "completed edit",
		Tags:    "auto,codex",
	})
	if err != nil {
		t.Fatalf("handle completion: %v", err)
	}
	if result.Status != AgentCompletionStatusCreated {
		t.Fatalf("expected status created, got %s", result.Status)
	}
	if result.CellID == nil {
		t.Fatalf("expected created cell id")
	}
	if result.Branch != "main" {
		t.Fatalf("expected main branch, got %s", result.Branch)
	}
	if result.Source != defaultAgentCompletionSource {
		t.Fatalf("expected default source %s, got %s", defaultAgentCompletionSource, result.Source)
	}

	run, err := svc.DB.GetAgentRun("run-created")
	if err != nil {
		t.Fatalf("get agent run: %v", err)
	}
	if run.Status != AgentCompletionStatusCreated {
		t.Fatalf("expected finalized status created, got %s", run.Status)
	}
}

func TestHandleAgentCompletionNoChange(t *testing.T) {
	svc := newTestService(t)
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := svc.CreateCell(context.Background(), SnapOptions{Message: "baseline", RunEval: false}); err != nil {
		t.Fatalf("create baseline cell: %v", err)
	}

	result, err := svc.HandleAgentCompletion(context.Background(), AgentCompletionOptions{
		RunID:   "run-no-change",
		Agent:   "claude",
		Message: "no file changes",
	})
	if err != nil {
		t.Fatalf("handle completion: %v", err)
	}
	if result.Status != AgentCompletionStatusNoChange {
		t.Fatalf("expected status no_change, got %s", result.Status)
	}
	if result.CellID != nil {
		t.Fatalf("expected no cell id for no_change, got %v", result.CellID)
	}

	run, err := svc.DB.GetAgentRun("run-no-change")
	if err != nil {
		t.Fatalf("get agent run: %v", err)
	}
	if run.Status != AgentCompletionStatusNoChange {
		t.Fatalf("expected finalized status no_change, got %s", run.Status)
	}
}

func TestHandleAgentCompletionDuplicate(t *testing.T) {
	svc := newTestService(t)
	if err := os.WriteFile(filepath.Join(svc.ProjectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	first, err := svc.HandleAgentCompletion(context.Background(), AgentCompletionOptions{
		RunID:   "run-duplicate",
		Agent:   "codex",
		Message: "completed once",
	})
	if err != nil {
		t.Fatalf("first completion: %v", err)
	}
	if first.Status != AgentCompletionStatusCreated {
		t.Fatalf("expected first status created, got %s", first.Status)
	}

	second, err := svc.HandleAgentCompletion(context.Background(), AgentCompletionOptions{
		RunID:   "run-duplicate",
		Agent:   "codex",
		Message: "completed again",
	})
	if err != nil {
		t.Fatalf("second completion duplicate: %v", err)
	}
	if second.Status != AgentCompletionStatusDuplicate {
		t.Fatalf("expected duplicate status, got %s", second.Status)
	}
	if first.CellID == nil || second.CellID == nil || *first.CellID != *second.CellID {
		t.Fatalf("expected duplicate response to reference prior cell: first=%v second=%v", first.CellID, second.CellID)
	}
}

func TestHandleAgentCompletionFailed(t *testing.T) {
	svc := newTestService(t)
	svc.ProjectDir = filepath.Join(svc.ProjectDir, "does-not-exist")

	result, err := svc.HandleAgentCompletion(context.Background(), AgentCompletionOptions{
		RunID:   "run-failed",
		Agent:   "codex",
		Message: "this will fail",
	})
	if err == nil {
		t.Fatalf("expected completion error")
	}
	if result.Status != AgentCompletionStatusFailed {
		t.Fatalf("expected failed status, got %s", result.Status)
	}
	if result.Error == "" {
		t.Fatalf("expected failed result to include error text")
	}

	run, getErr := svc.DB.GetAgentRun("run-failed")
	if getErr != nil {
		t.Fatalf("get failed run: %v", getErr)
	}
	if run.Status != AgentCompletionStatusFailed {
		t.Fatalf("expected finalized failed status, got %s", run.Status)
	}
	if run.Error == nil || *run.Error == "" {
		t.Fatalf("expected stored run error text, got %v", run.Error)
	}
}
