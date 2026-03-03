package eval

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prittamravi/converge/internal/config"
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

func TestRunConfiguredCommandsOverridesAutoDetect(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner()
	runner.SetPolicy(config.EvalPolicy{
		Tests: []string{"echo ok"},
	})

	result, err := runner.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("run configured checks: %v", err)
	}
	if !result.HasTests {
		t.Fatalf("expected tests to be marked as run")
	}
	if result.TestsPassed != 1 || result.TestsFailed != 0 {
		t.Fatalf("unexpected configured test result: %+v", result)
	}
	for _, skipped := range result.Skipped {
		if strings.Contains(skipped, "no-project-detected") {
			t.Fatalf("did not expect project auto-detect skip in override mode")
		}
	}
}

func TestRunConfiguredCommandsMissingCommandIsSkipped(t *testing.T) {
	dir := t.TempDir()
	runner := NewRunner()
	runner.SetPolicy(config.EvalPolicy{
		Tests: []string{"converge_nonexistent_command_xyz"},
	})

	result, err := runner.Run(context.Background(), dir)
	if err != nil {
		t.Fatalf("run configured checks: %v", err)
	}
	if !result.HasTests {
		t.Fatalf("expected tests flag to be true")
	}
	if result.TestsFailed != 0 {
		t.Fatalf("expected missing command to be skipped, got failures=%d", result.TestsFailed)
	}
	if len(result.Skipped) == 0 {
		t.Fatalf("expected skipped entry for missing configured command")
	}
}
