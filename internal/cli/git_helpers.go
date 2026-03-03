package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

func runGitCommand(projectDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed == "" {
			return "", fmt.Errorf("git %s failed: %w", strings.Join(args, " "), err)
		}
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, trimmed)
	}
	return trimmed, nil
}

func resolveGitHooksDir(projectDir string) (string, error) {
	hooksPath, err := runGitCommand(projectDir, "rev-parse", "--git-path", "hooks")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(hooksPath) {
		return hooksPath, nil
	}
	return filepath.Join(projectDir, hooksPath), nil
}
