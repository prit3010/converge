package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunGitHooksInstallPreservesExistingAndIsIdempotent(t *testing.T) {
	requireGitForCLI(t)
	projectDir := t.TempDir()
	initGitRepoForCLI(t, projectDir)

	hooksDir, err := resolveGitHooksDir(projectDir)
	if err != nil {
		t.Fatalf("resolve hooks dir: %v", err)
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		t.Fatalf("mkdir hooks dir: %v", err)
	}

	targetHook := filepath.Join(hooksDir, "post-commit")
	originalContent := "#!/usr/bin/env bash\necho user hook\n"
	if err := os.WriteFile(targetHook, []byte(originalContent), 0o755); err != nil {
		t.Fatalf("write original post-commit: %v", err)
	}

	var out bytes.Buffer
	if err := runGitHooksInstall(projectDir, &out); err != nil {
		t.Fatalf("install git hooks: %v", err)
	}

	convergeBin := "converge"
	if resolved, err := os.Executable(); err == nil && strings.TrimSpace(resolved) != "" {
		convergeBin = filepath.Clean(resolved)
	}
	scriptPath := filepath.Join(projectDir, hookScriptsDirName, "claude-post-response-hook.sh")
	expectedCommand := "CONVERGE_BIN=" + convergeBin + " CONVERGE_PROJECT_DIR=" + projectDir + " " + scriptPath

	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	settings := loadClaudeSettingsForCLI(t, settingsPath)
	if countClaudeEventCommand(settings, "Stop", expectedCommand) != 1 {
		t.Fatalf("expected Stop event command installed exactly once")
	}
	if countClaudeEventCommand(settings, "SessionEnd", expectedCommand) != 1 {
		t.Fatalf("expected SessionEnd event command installed exactly once")
	}

	for _, scriptName := range []string{"claude-post-response-hook.sh", "converge-post-commit-hook.sh"} {
		script := filepath.Join(projectDir, hookScriptsDirName, scriptName)
		info, err := os.Stat(script)
		if err != nil {
			t.Fatalf("stat script %s: %v", scriptName, err)
		}
		if info.IsDir() {
			t.Fatalf("expected file for script %s", scriptName)
		}
		if info.Mode()&0o111 == 0 {
			t.Fatalf("expected script %s to be executable, mode=%o", scriptName, info.Mode().Perm())
		}
	}

	legacyScriptsDir := filepath.Join(projectDir, legacyScriptsDirName)
	if _, err := os.Stat(legacyScriptsDir); !os.IsNotExist(err) {
		t.Fatalf("expected legacy scripts dir to remain absent for fresh installs")
	}

	preservedPath := filepath.Join(hooksDir, preservedPostCommitHook)
	preservedContent, err := os.ReadFile(preservedPath)
	if err != nil {
		t.Fatalf("read preserved hook: %v", err)
	}
	if string(preservedContent) != originalContent {
		t.Fatalf("preserved hook content mismatch:\nwant=%q\ngot=%q", originalContent, string(preservedContent))
	}

	managedContent, err := os.ReadFile(targetHook)
	if err != nil {
		t.Fatalf("read managed post-commit: %v", err)
	}
	managed := string(managedContent)
	if !strings.Contains(managed, managedPostCommitMarker) {
		t.Fatalf("expected managed marker in post-commit hook")
	}
	if !strings.Contains(managed, preservedPostCommitHook) {
		t.Fatalf("expected managed hook to chain preserved hook")
	}

	if err := runGitHooksInstall(projectDir, &out); err != nil {
		t.Fatalf("reinstall git hooks: %v", err)
	}

	preservedContentAfter, err := os.ReadFile(preservedPath)
	if err != nil {
		t.Fatalf("read preserved hook after reinstall: %v", err)
	}
	if string(preservedContentAfter) != originalContent {
		t.Fatalf("preserved hook changed after reinstall")
	}

	settings = loadClaudeSettingsForCLI(t, settingsPath)
	if countClaudeEventCommand(settings, "Stop", expectedCommand) != 1 {
		t.Fatalf("expected Stop event command to remain single entry after reinstall")
	}
	if countClaudeEventCommand(settings, "SessionEnd", expectedCommand) != 1 {
		t.Fatalf("expected SessionEnd event command to remain single entry after reinstall")
	}
}

func TestRunGitHooksInstallMigratesLegacyClaudeScript(t *testing.T) {
	requireGitForCLI(t)
	projectDir := t.TempDir()
	initGitRepoForCLI(t, projectDir)

	legacyScriptPath := filepath.Join(projectDir, legacyScriptsDirName, "claude-post-response-hook.sh")
	if err := os.MkdirAll(filepath.Dir(legacyScriptPath), 0o755); err != nil {
		t.Fatalf("mkdir scripts dir: %v", err)
	}
	existingScript := "#!/usr/bin/env bash\necho custom\n"
	if err := os.WriteFile(legacyScriptPath, []byte(existingScript), 0o755); err != nil {
		t.Fatalf("write custom claude hook script: %v", err)
	}

	if err := runGitHooksInstall(projectDir, io.Discard); err != nil {
		t.Fatalf("install git hooks: %v", err)
	}

	scriptPath := filepath.Join(projectDir, hookScriptsDirName, "claude-post-response-hook.sh")
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read custom claude hook script: %v", err)
	}
	if string(content) != existingScript {
		t.Fatalf("expected claude hook script content to be migrated from legacy scripts path")
	}
}

func TestRunGitHooksInstallOutsideGitRepoDoesNotCreateScripts(t *testing.T) {
	projectDir := t.TempDir()

	if err := runGitHooksInstall(projectDir, io.Discard); err == nil {
		t.Fatalf("expected install to fail outside git repo")
	}

	for _, dirName := range []string{hookScriptsDirName, legacyScriptsDirName} {
		dirPath := filepath.Join(projectDir, dirName)
		if _, err := os.Stat(dirPath); !os.IsNotExist(err) {
			t.Fatalf("expected %s to not be created outside git repo", dirPath)
		}
	}
}

func TestRunGitHooksInstallGitOnlySkipsClaudeSettings(t *testing.T) {
	requireGitForCLI(t)
	projectDir := t.TempDir()
	initGitRepoForCLI(t, projectDir)

	if err := runGitHooksInstallGit(projectDir, io.Discard); err != nil {
		t.Fatalf("install git hooks only: %v", err)
	}

	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	if _, err := os.Stat(settingsPath); !os.IsNotExist(err) {
		t.Fatalf("expected no claude settings for git-only install")
	}

	postCommitScript := filepath.Join(projectDir, hookScriptsDirName, "converge-post-commit-hook.sh")
	if _, err := os.Stat(postCommitScript); err != nil {
		t.Fatalf("expected post-commit hook script to be installed for git-only install: %v", err)
	}
	claudeScript := filepath.Join(projectDir, hookScriptsDirName, "claude-post-response-hook.sh")
	if _, err := os.Stat(claudeScript); !os.IsNotExist(err) {
		t.Fatalf("expected claude hook script to remain absent for git-only install")
	}
}

func TestRunClaudeHooksInstallOnlyCreatesClaudeSettings(t *testing.T) {
	projectDir := t.TempDir()

	if err := runClaudeHooksInstall(projectDir, io.Discard); err != nil {
		t.Fatalf("install claude hooks only: %v", err)
	}

	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	settings := loadClaudeSettingsForCLI(t, settingsPath)
	if hooksRoot, ok := settings["hooks"].(map[string]any); !ok || len(hooksRoot) == 0 {
		t.Fatalf("expected hooks config to be written for claude install")
	}

	claudeScript := filepath.Join(projectDir, hookScriptsDirName, "claude-post-response-hook.sh")
	if _, err := os.Stat(claudeScript); err != nil {
		t.Fatalf("expected claude hook script to be installed: %v", err)
	}
	postCommitScript := filepath.Join(projectDir, hookScriptsDirName, "converge-post-commit-hook.sh")
	if _, err := os.Stat(postCommitScript); !os.IsNotExist(err) {
		t.Fatalf("expected post-commit script to remain absent for claude-only install")
	}

	hooksDir := filepath.Join(projectDir, ".git", "hooks")
	if _, err := os.Stat(hooksDir); !os.IsNotExist(err) {
		t.Fatalf("expected no git hook directory for claude-only install")
	}
}

func TestRunClaudeHooksInstallRemovesLegacyScriptCommand(t *testing.T) {
	projectDir := t.TempDir()
	convergeBin := "converge"
	if resolved, err := os.Executable(); err == nil && strings.TrimSpace(resolved) != "" {
		convergeBin = filepath.Clean(resolved)
	}

	legacyScriptPath := filepath.Join(projectDir, legacyScriptsDirName, "claude-post-response-hook.sh")
	if err := os.MkdirAll(filepath.Dir(legacyScriptPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy scripts dir: %v", err)
	}
	if err := os.WriteFile(legacyScriptPath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write legacy script: %v", err)
	}

	legacyCommand := "CONVERGE_BIN=" + convergeBin + " CONVERGE_PROJECT_DIR=" + projectDir + " " + legacyScriptPath
	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		t.Fatalf("mkdir claude dir: %v", err)
	}
	raw := `{
  "hooks": {
    "Stop": [{"matcher":"*","hooks":[{"type":"command","command":"` + legacyCommand + `"}]}],
    "SessionEnd": [{"matcher":"*","hooks":[{"type":"command","command":"` + legacyCommand + `"}]}]
  }
}`
	if err := os.WriteFile(settingsPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write legacy settings: %v", err)
	}

	if err := runClaudeHooksInstall(projectDir, io.Discard); err != nil {
		t.Fatalf("install claude hooks only: %v", err)
	}

	newCommand := "CONVERGE_BIN=" + convergeBin + " CONVERGE_PROJECT_DIR=" + projectDir + " " + filepath.Join(projectDir, hookScriptsDirName, "claude-post-response-hook.sh")
	settings := loadClaudeSettingsForCLI(t, settingsPath)
	if countClaudeEventCommand(settings, "Stop", legacyCommand) != 0 {
		t.Fatalf("expected legacy Stop command to be removed")
	}
	if countClaudeEventCommand(settings, "SessionEnd", legacyCommand) != 0 {
		t.Fatalf("expected legacy SessionEnd command to be removed")
	}
	if countClaudeEventCommand(settings, "Stop", newCommand) != 1 {
		t.Fatalf("expected new Stop command to be installed once")
	}
	if countClaudeEventCommand(settings, "SessionEnd", newCommand) != 1 {
		t.Fatalf("expected new SessionEnd command to be installed once")
	}
}

func TestRunHookGitCommitIncludesReplayOnFailure(t *testing.T) {
	requireGitForCLI(t)
	projectDir := t.TempDir()
	initGitRepoForCLI(t, projectDir)

	if err := runInit(projectDir); err != nil {
		t.Fatalf("run init: %v", err)
	}

	tracked := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(tracked, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write tracked file: %v", err)
	}
	sha := commitAllForCLI(t, projectDir, "initial")
	branch := gitOutputForCLI(t, projectDir, "branch", "--show-current")

	if err := runSnap(projectDir, "baseline", "", "", false, false, io.Discard); err != nil {
		t.Fatalf("run snap: %v", err)
	}

	lockPath := filepath.Join(projectDir, ".converge", "archive.lock")
	if err := os.WriteFile(lockPath, []byte("locked"), 0o644); err != nil {
		t.Fatalf("write archive lock: %v", err)
	}

	var out bytes.Buffer
	err := runHookGitCommit(projectDir, hookGitCommitFlags{
		SHA:     sha,
		Branch:  branch,
		Subject: "initial",
	}, &out)
	if err == nil {
		t.Fatalf("expected hook git-commit to fail when archive lock exists")
	}
	msg := err.Error()
	if !strings.Contains(msg, "replay: converge hook git-commit") {
		t.Fatalf("expected replay instruction in error, got: %s", msg)
	}
}

func requireGitForCLI(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required for git-hook tests")
	}
}

func initGitRepoForCLI(t *testing.T, dir string) {
	t.Helper()
	runGitForCLI(t, dir, "init")
	runGitForCLI(t, dir, "config", "user.email", "converge-tests@example.com")
	runGitForCLI(t, dir, "config", "user.name", "Converge Tests")
}

func commitAllForCLI(t *testing.T, dir string, subject string) string {
	t.Helper()
	runGitForCLI(t, dir, "add", "-A")
	runGitForCLI(t, dir, "commit", "-m", subject)
	return gitOutputForCLI(t, dir, "rev-parse", "HEAD")
}

func runGitForCLI(t *testing.T, dir string, args ...string) {
	t.Helper()
	_ = gitOutputForCLI(t, dir, args...)
}

func gitOutputForCLI(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func loadClaudeSettingsForCLI(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read claude settings: %v", err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("decode claude settings: %v", err)
	}
	return settings
}

func countClaudeEventCommand(settings map[string]any, event, command string) int {
	hooksRoot, ok := settings["hooks"].(map[string]any)
	if !ok {
		return 0
	}
	events, ok := hooksRoot[event].([]any)
	if !ok {
		return 0
	}
	count := 0
	for _, item := range events {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hooks, ok := entry["hooks"].([]any)
		if !ok {
			continue
		}
		for _, hookItem := range hooks {
			hook, ok := hookItem.(map[string]any)
			if !ok {
				continue
			}
			hookType, _ := hook["type"].(string)
			hookCommand, _ := hook["command"].(string)
			if hookType == "command" && hookCommand == command {
				count++
			}
		}
	}
	return count
}
