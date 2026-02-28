package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProjects(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x"), 0o644); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "package.json"), []byte("{}"), 0o644); err != nil {
		t.Fatalf("write package.json: %v", err)
	}
	projects := DetectProjects(dir)
	if len(projects) != 2 {
		t.Fatalf("expected 2 project types, got %d", len(projects))
	}
}

func TestParseGoTestOutput(t *testing.T) {
	out := "{\"Action\":\"pass\",\"Test\":\"TestA\"}\n{\"Action\":\"fail\",\"Test\":\"TestB\"}"
	passed, failed := parseGoTestOutput(out)
	if passed != 1 || failed != 1 {
		t.Fatalf("unexpected parse result passed=%d failed=%d", passed, failed)
	}
}

func TestParsePytestSummary(t *testing.T) {
	passed, failed := parsePytestSummary("=== 4 passed, 2 failed in 1.23s ===")
	if passed != 4 || failed != 2 {
		t.Fatalf("unexpected parse result passed=%d failed=%d", passed, failed)
	}
}

func TestRunGoChecksConservativeFailureAccounting(t *testing.T) {
	toolsDir := t.TempDir()
	projectDir := t.TempDir()

	writeExecutable(t, filepath.Join(toolsDir, "go"), "#!/usr/bin/env bash\nexit 1\n")
	writeExecutable(t, filepath.Join(toolsDir, "golangci-lint"), "#!/usr/bin/env bash\nexit 1\n")
	t.Setenv("PATH", toolsDir)

	var result Result
	runGoChecks(context.Background(), projectDir, &result)

	if !result.HasTests {
		t.Fatalf("expected tests to be marked as run")
	}
	if result.TestsFailed != 1 {
		t.Fatalf("expected conservative tests_failed=1, got %d", result.TestsFailed)
	}
	if !result.HasLint {
		t.Fatalf("expected lint to be marked as run")
	}
	if result.LintErrors != 1 {
		t.Fatalf("expected conservative lint_errors=1, got %d", result.LintErrors)
	}
}

func TestRunPythonChecksConservativeFailureAccounting(t *testing.T) {
	toolsDir := t.TempDir()
	projectDir := t.TempDir()

	writeExecutable(t, filepath.Join(toolsDir, "pytest"), "#!/usr/bin/env bash\nexit 1\n")
	writeExecutable(t, filepath.Join(toolsDir, "ruff"), "#!/usr/bin/env bash\nexit 1\n")
	writeExecutable(t, filepath.Join(toolsDir, "mypy"), "#!/usr/bin/env bash\nexit 1\n")
	t.Setenv("PATH", toolsDir)

	var result Result
	runPythonChecks(context.Background(), projectDir, &result)

	if !result.HasTests || result.TestsFailed != 1 {
		t.Fatalf("expected conservative python test failure accounting, got %+v", result)
	}
	if !result.HasLint || result.LintErrors != 1 {
		t.Fatalf("expected conservative python lint accounting, got %+v", result)
	}
	if !result.HasTypes || result.TypeErrors != 1 {
		t.Fatalf("expected conservative python type accounting, got %+v", result)
	}
}

func writeExecutable(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable %s: %v", path, err)
	}
}
