package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHookCompleteValidatesRequiredFlags(t *testing.T) {
	projectDir := t.TempDir()
	var out bytes.Buffer
	err := runHookComplete(projectDir, hookCompleteFlags{
		Agent:   "codex",
		Message: "done",
	}, &out)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	if !strings.Contains(err.Error(), "missing required flag: --run-id") {
		t.Fatalf("unexpected validation error: %v", err)
	}
}

func TestRunHookCompleteJSONOutput(t *testing.T) {
	projectDir := t.TempDir()
	if err := runInit(projectDir); err != nil {
		t.Fatalf("run init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	err := runHookComplete(projectDir, hookCompleteFlags{
		RunID:      "run-json-1",
		Agent:      "codex",
		Message:    "completed",
		Tags:       "auto,codex",
		OutputJSON: true,
	}, &out)
	if err != nil {
		t.Fatalf("run hook complete: %v", err)
	}

	var payload map[string]any
	if decErr := json.Unmarshal(out.Bytes(), &payload); decErr != nil {
		t.Fatalf("decode json output: %v\nraw=%s", decErr, out.String())
	}
	if payload["status"] != "created" {
		t.Fatalf("expected created status, got %v", payload["status"])
	}
	if payload["run_id"] != "run-json-1" {
		t.Fatalf("expected run_id run-json-1, got %v", payload["run_id"])
	}
	if payload["source"] != "agent_complete" {
		t.Fatalf("expected default source agent_complete, got %v", payload["source"])
	}
	if _, ok := payload["cell_id"]; !ok {
		t.Fatalf("expected cell_id in json output: %v", payload)
	}
}
