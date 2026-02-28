package eval

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type ProjectType string

const (
	ProjectGo     ProjectType = "go"
	ProjectPython ProjectType = "python"
	ProjectNode   ProjectType = "node"
)

type Result struct {
	TestsPassed int
	TestsFailed int
	LintErrors  int
	TypeErrors  int

	HasTests bool
	HasLint  bool
	HasTypes bool

	Skipped []string
}

func (r Result) TestsPassedPtr() *int {
	if !r.HasTests {
		return nil
	}
	v := r.TestsPassed
	return &v
}

func (r Result) TestsFailedPtr() *int {
	if !r.HasTests {
		return nil
	}
	v := r.TestsFailed
	return &v
}

func (r Result) LintErrorsPtr() *int {
	if !r.HasLint {
		return nil
	}
	v := r.LintErrors
	return &v
}

func (r Result) TypeErrorsPtr() *int {
	if !r.HasTypes {
		return nil
	}
	v := r.TypeErrors
	return &v
}

func (r Result) SkippedPtr() *string {
	if len(r.Skipped) == 0 {
		return nil
	}
	joined := strings.Join(r.Skipped, ",")
	return &joined
}

type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

func (r *Runner) Run(ctx context.Context, projectDir string) (Result, error) {
	res := Result{}
	projects := DetectProjects(projectDir)
	if len(projects) == 0 {
		res.Skipped = append(res.Skipped, "no-project-detected")
		return res, nil
	}

	for _, project := range projects {
		switch project {
		case ProjectGo:
			runGoChecks(ctx, projectDir, &res)
		case ProjectPython:
			runPythonChecks(ctx, projectDir, &res)
		case ProjectNode:
			runNodeChecks(ctx, projectDir, &res)
		}
	}

	return res, nil
}

func DetectProjects(dir string) []ProjectType {
	out := make([]ProjectType, 0, 3)
	if exists(filepath.Join(dir, "go.mod")) {
		out = append(out, ProjectGo)
	}
	if exists(filepath.Join(dir, "pyproject.toml")) || exists(filepath.Join(dir, "setup.py")) || exists(filepath.Join(dir, "pytest.ini")) {
		out = append(out, ProjectPython)
	}
	if exists(filepath.Join(dir, "package.json")) {
		out = append(out, ProjectNode)
	}
	return out
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func runGoChecks(ctx context.Context, dir string, res *Result) {
	if toolExists("go") {
		out, err := runCmd(ctx, dir, "go", "test", "-json", "./...")
		passed, failed := parseGoTestOutput(out)
		if err != nil && failed == 0 {
			failed = 1
		}
		res.HasTests = true
		res.TestsPassed += passed
		res.TestsFailed += failed
	} else {
		res.Skipped = append(res.Skipped, "go")
	}

	if toolExists("golangci-lint") {
		out, err := runCmd(ctx, dir, "golangci-lint", "run", "./...")
		res.HasLint = true
		res.LintErrors += conservativeProblemCount(out, err)
	} else {
		res.Skipped = append(res.Skipped, "golangci-lint")
	}
}

func runPythonChecks(ctx context.Context, dir string, res *Result) {
	if toolExists("pytest") {
		out, err := runCmd(ctx, dir, "pytest", "-q", "--tb=no")
		passed, failed := parsePytestSummary(out)
		if err != nil && failed == 0 {
			failed = 1
		}
		res.HasTests = true
		res.TestsPassed += passed
		res.TestsFailed += failed
	} else {
		res.Skipped = append(res.Skipped, "pytest")
	}

	if toolExists("ruff") {
		out, err := runCmd(ctx, dir, "ruff", "check", ".")
		res.HasLint = true
		res.LintErrors += conservativeProblemCount(out, err)
	} else {
		res.Skipped = append(res.Skipped, "ruff")
	}

	if toolExists("mypy") {
		out, err := runCmd(ctx, dir, "mypy", ".")
		res.HasTypes = true
		res.TypeErrors += conservativeProblemCount(out, err)
	} else {
		res.Skipped = append(res.Skipped, "mypy")
	}
}

func runNodeChecks(ctx context.Context, dir string, res *Result) {
	if toolExists("npm") {
		_, err := runCmd(ctx, dir, "npm", "test", "--silent")
		res.HasTests = true
		if err != nil {
			res.TestsFailed += 1
		} else {
			res.TestsPassed += 1
		}
	} else {
		res.Skipped = append(res.Skipped, "npm")
	}

	if toolExists("npx") {
		outLint, lintErr := runCmd(ctx, dir, "npx", "eslint", ".")
		res.HasLint = true
		res.LintErrors += conservativeProblemCount(outLint, lintErr)

		outTypes, typeErr := runCmd(ctx, dir, "npx", "tsc", "--noEmit")
		res.HasTypes = true
		res.TypeErrors += conservativeProblemCount(outTypes, typeErr)
	} else {
		res.Skipped = append(res.Skipped, "npx")
	}
}

func toolExists(name string) bool {
	_, ok := resolveTool(name)
	return ok
}

func runCmd(ctx context.Context, dir string, name string, args ...string) (string, error) {
	tool, ok := resolveTool(name)
	if !ok {
		return "", fmt.Errorf("tool %s not found", name)
	}
	cmd := exec.CommandContext(ctx, tool, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func resolveTool(name string) (string, bool) {
	path, err := exec.LookPath(name)
	if err == nil {
		return path, true
	}
	if name == "go" {
		candidates := []string{"/usr/local/go/bin/go", "/opt/homebrew/bin/go"}
		for _, candidate := range candidates {
			if _, statErr := os.Stat(candidate); statErr == nil {
				return candidate, true
			}
		}
	}
	return "", false
}

func countProblemLines(output string) int {
	output = strings.TrimSpace(output)
	if output == "" {
		return 0
	}
	count := 0
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func conservativeProblemCount(output string, cmdErr error) int {
	count := countProblemLines(output)
	if cmdErr != nil && count == 0 {
		return 1
	}
	return count
}

func parseGoTestOutput(output string) (passed int, failed int) {
	for _, line := range strings.Split(output, "\n") {
		if strings.Contains(line, `"Action":"pass"`) && strings.Contains(line, `"Test":"`) {
			passed++
		}
		if strings.Contains(line, `"Action":"fail"`) && strings.Contains(line, `"Test":"`) {
			failed++
		}
	}
	return passed, failed
}

var passedRe = regexp.MustCompile(`([0-9]+)\s+passed`)
var failedRe = regexp.MustCompile(`([0-9]+)\s+failed`)

func parsePytestSummary(output string) (int, int) {
	passed := 0
	failed := 0
	if m := passedRe.FindStringSubmatch(output); len(m) == 2 {
		fmt.Sscanf(m[1], "%d", &passed)
	}
	if m := failedRe.FindStringSubmatch(output); len(m) == 2 {
		fmt.Sscanf(m[1], "%d", &failed)
	}
	return passed, failed
}
