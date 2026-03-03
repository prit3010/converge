package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/prittamravi/converge/internal/config"
)

func TestRunSnapJSONEnvelope(t *testing.T) {
	projectDir := t.TempDir()
	if err := runInit(projectDir); err != nil {
		t.Fatalf("run init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write main.go: %v", err)
	}

	var out bytes.Buffer
	if err := runSnap(projectDir, "json snap", "", "", false, true, &out); err != nil {
		t.Fatalf("run snap json: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode envelope: %v\nraw=%s", err, out.String())
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	if payload["command"] != "snap" {
		t.Fatalf("expected command snap, got %v", payload["command"])
	}
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected meta object, got %T", payload["meta"])
	}
	if meta["schema_version"] != config.DefaultJSONVersion {
		t.Fatalf("expected schema version %s, got %v", config.DefaultJSONVersion, meta["schema_version"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if _, ok := data["cell"]; !ok {
		t.Fatalf("expected cell in data payload: %+v", data)
	}
}

func TestWriteCommandErrorJSONEnvelope(t *testing.T) {
	errPayload := classifyCommandError(notFoundErrorf("cell c_999999 not found"))
	var out bytes.Buffer
	if err := writeCommandErrorJSON(&out, "diff", errPayload); err != nil {
		t.Fatalf("write error envelope: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode error envelope: %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %v", payload["ok"])
	}
	if payload["command"] != "diff" {
		t.Fatalf("expected command diff, got %v", payload["command"])
	}
	errObj, ok := payload["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %T", payload["error"])
	}
	if errObj["code"] != string(ErrorCodeNotFound) {
		t.Fatalf("expected error code %s, got %v", ErrorCodeNotFound, errObj["code"])
	}
	if errObj["exit_code"] != float64(exitCodeForError(ErrorCodeNotFound)) {
		t.Fatalf("unexpected exit code: %v", errObj["exit_code"])
	}
}
