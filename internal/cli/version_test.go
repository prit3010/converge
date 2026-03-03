package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestRunVersionText(t *testing.T) {
	prevVersion := Version
	prevCommit := Commit
	prevBuildDate := BuildDate
	t.Cleanup(func() {
		Version = prevVersion
		Commit = prevCommit
		BuildDate = prevBuildDate
	})

	Version = "v0.1.0"
	Commit = "abc1234"
	BuildDate = "2026-03-03T00:00:00Z"

	var out bytes.Buffer
	if err := runVersion(false, &out); err != nil {
		t.Fatalf("run version text: %v", err)
	}

	raw := out.String()
	if !strings.Contains(raw, "converge v0.1.0") {
		t.Fatalf("expected version line, got:\n%s", raw)
	}
	if !strings.Contains(raw, "commit: abc1234") {
		t.Fatalf("expected commit line, got:\n%s", raw)
	}
	if !strings.Contains(raw, "build date: 2026-03-03T00:00:00Z") {
		t.Fatalf("expected build date line, got:\n%s", raw)
	}
}

func TestRunVersionJSONEnvelope(t *testing.T) {
	prevVersion := Version
	prevCommit := Commit
	prevBuildDate := BuildDate
	t.Cleanup(func() {
		Version = prevVersion
		Commit = prevCommit
		BuildDate = prevBuildDate
	})

	Version = "v0.1.0"
	Commit = "abc1234"
	BuildDate = "2026-03-03T00:00:00Z"

	var out bytes.Buffer
	if err := runVersion(true, &out); err != nil {
		t.Fatalf("run version json: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("decode version json: %v\nraw=%s", err, out.String())
	}
	if payload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", payload["ok"])
	}
	if payload["command"] != "version" {
		t.Fatalf("expected command version, got %v", payload["command"])
	}
	data, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", payload["data"])
	}
	if data["version"] != "v0.1.0" {
		t.Fatalf("expected version v0.1.0, got %v", data["version"])
	}
	if data["commit"] != "abc1234" {
		t.Fatalf("expected commit abc1234, got %v", data["commit"])
	}
	if data["build_date"] != "2026-03-03T00:00:00Z" {
		t.Fatalf("expected build_date value, got %v", data["build_date"])
	}
}
