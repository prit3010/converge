package cli

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const (
	managedPostCommitMarker = "CONVERGE_MANAGED_POST_COMMIT"
	preservedPostCommitHook = "post-commit.user"
	claudeSettingsPath      = ".claude/settings.local.json"
	hookScriptsDirName      = "converge_scripts"
	legacyScriptsDirName    = "scripts"
)

var (
	//go:embed hook_scripts/*.sh
	embeddedHookScripts embed.FS
)

func newGitHooksCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "hooks",
		Aliases: []string{"git-hooks"},
		Short:   "Manage git and Claude hook integrations",
	}
	cmd.AddCommand(newGitHooksInstallCmd())
	cmd.AddCommand(newGitHooksInstallGitCmd())
	cmd.AddCommand(newGitHooksInstallClaudeCmd())
	return cmd
}

func newGitHooksInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install both git and Claude hook integrations",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runGitHooksInstall(cwd, cmd.OutOrStdout())
		},
	}
}

func newGitHooksInstallGitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install-git",
		Short: "Install only managed git post-commit hook integration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runGitHooksInstallGit(cwd, cmd.OutOrStdout())
		},
	}
}

func newGitHooksInstallClaudeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install-claude",
		Short: "Install only Claude hook integration",
		RunE: func(cmd *cobra.Command, args []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			return runClaudeHooksInstall(cwd, cmd.OutOrStdout())
		},
	}
}

func runGitHooksInstall(projectDir string, out io.Writer) error {
	if err := runGitHooksInstallGit(projectDir, out); err != nil {
		return err
	}
	return runClaudeHooksInstall(projectDir, out)
}

func runGitHooksInstallGit(projectDir string, out io.Writer) error {
	hooksDir, err := resolveGitHooksDir(projectDir)
	if err != nil {
		return err
	}
	if _, err := ensureProjectHookScript(projectDir, "converge-post-commit-hook.sh", out); err != nil {
		return err
	}
	if err := os.MkdirAll(hooksDir, 0o755); err != nil {
		return fmt.Errorf("create hooks directory: %w", err)
	}

	targetHook := filepath.Join(hooksDir, "post-commit")
	preservedHook := filepath.Join(hooksDir, preservedPostCommitHook)

	existing, readErr := os.ReadFile(targetHook)
	if readErr == nil {
		if !isManagedPostCommit(existing) {
			if _, statErr := os.Stat(preservedHook); os.IsNotExist(statErr) {
				if err := os.Rename(targetHook, preservedHook); err != nil {
					return fmt.Errorf("preserve existing post-commit hook: %w", err)
				}
				fmt.Fprintf(out, "Preserved existing post-commit hook at %s\n", preservedHook)
			} else {
				backupPath := preservedHook + "." + time.Now().UTC().Format("20060102T150405Z")
				if err := os.Rename(targetHook, backupPath); err != nil {
					return fmt.Errorf("backup extra post-commit hook: %w", err)
				}
				fmt.Fprintf(out, "Preserved additional post-commit hook at %s\n", backupPath)
			}
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read existing post-commit hook: %w", readErr)
	}

	wrapper := managedPostCommitWrapper()
	if err := os.WriteFile(targetHook, []byte(wrapper), 0o755); err != nil {
		return fmt.Errorf("write managed post-commit hook: %w", err)
	}

	fmt.Fprintf(out, "Installed Converge post-commit hook at %s\n", targetHook)
	fmt.Fprintf(out, "Managed hook chains preserved hook at %s when present\n", preservedHook)
	return nil
}

func ensureProjectHookScript(projectDir, scriptName string, out io.Writer) (string, error) {
	scriptPath := filepath.Join(projectDir, hookScriptsDirName, scriptName)
	info, err := os.Stat(scriptPath)
	if err == nil {
		if info.IsDir() {
			return "", fmt.Errorf("hook script path is a directory: %s", scriptPath)
		}
		return scriptPath, nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("stat hook script %s: %w", scriptPath, err)
	}

	var content []byte
	legacyPath := filepath.Join(projectDir, legacyScriptsDirName, scriptName)
	legacyInfo, legacyErr := os.Stat(legacyPath)
	switch {
	case legacyErr == nil:
		if legacyInfo.IsDir() {
			return "", fmt.Errorf("legacy hook script path is a directory: %s", legacyPath)
		}
		content, err = os.ReadFile(legacyPath)
		if err != nil {
			return "", fmt.Errorf("read legacy hook script %s: %w", legacyPath, err)
		}
	case os.IsNotExist(legacyErr):
		content, err = embeddedHookScripts.ReadFile(filepath.Join("hook_scripts", scriptName))
		if err != nil {
			return "", fmt.Errorf("load embedded hook script %s: %w", scriptName, err)
		}
	default:
		return "", fmt.Errorf("stat legacy hook script %s: %w", legacyPath, legacyErr)
	}
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return "", fmt.Errorf("create hook scripts directory: %w", err)
	}
	if err := os.WriteFile(scriptPath, content, 0o755); err != nil {
		return "", fmt.Errorf("write hook script %s: %w", scriptPath, err)
	}
	if legacyErr == nil {
		fmt.Fprintf(out, "Migrated Converge hook script from %s to %s\n", legacyPath, scriptPath)
	} else {
		fmt.Fprintf(out, "Installed Converge hook script at %s\n", scriptPath)
	}
	return scriptPath, nil
}

func isManagedPostCommit(content []byte) bool {
	return strings.Contains(string(content), managedPostCommitMarker)
}

func managedPostCommitWrapper() string {
	return fmt.Sprintf(`#!/usr/bin/env bash
set -uo pipefail
# %s

HOOK_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
USER_HOOK="$HOOK_DIR/%s"
USER_STATUS=0

if [[ -f "$USER_HOOK" ]]; then
  if [[ -x "$USER_HOOK" ]]; then
    "$USER_HOOK" "$@" || USER_STATUS=$?
  else
    bash "$USER_HOOK" "$@" || USER_STATUS=$?
  fi
fi

PROJECT_DIR="$(git rev-parse --show-toplevel 2>/dev/null || pwd)"
HOOK_SCRIPT="$PROJECT_DIR/%s/converge-post-commit-hook.sh"
if [[ ! -f "$HOOK_SCRIPT" ]]; then
  echo "converge post-commit hook script missing: $HOOK_SCRIPT" >&2
  exit 1
fi

bash "$HOOK_SCRIPT" "$@"
CONVERGE_STATUS=$?
if [[ $CONVERGE_STATUS -ne 0 ]]; then
  exit $CONVERGE_STATUS
fi

if [[ $USER_STATUS -ne 0 ]]; then
  exit $USER_STATUS
fi
	`, managedPostCommitMarker, preservedPostCommitHook, hookScriptsDirName)
}

func runClaudeHooksInstall(projectDir string, out io.Writer) error {
	scriptPath, err := ensureProjectHookScript(projectDir, "claude-post-response-hook.sh", out)
	if err != nil {
		return err
	}

	convergeBin := "converge"
	if resolved, binErr := os.Executable(); binErr == nil && strings.TrimSpace(resolved) != "" {
		convergeBin = filepath.Clean(resolved)
	}

	command := fmt.Sprintf("CONVERGE_BIN=%s CONVERGE_PROJECT_DIR=%s %s", convergeBin, projectDir, scriptPath)
	legacyScriptPath := filepath.Join(projectDir, legacyScriptsDirName, "claude-post-response-hook.sh")
	settingsPath := filepath.Join(projectDir, claudeSettingsPath)
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o755); err != nil {
		return fmt.Errorf("create .claude directory: %w", err)
	}

	root := map[string]any{}
	data, readErr := os.ReadFile(settingsPath)
	if readErr == nil {
		if strings.TrimSpace(string(data)) != "" {
			if err := json.Unmarshal(data, &root); err != nil {
				return fmt.Errorf("parse claude settings %s: %w", settingsPath, err)
			}
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("read claude settings %s: %w", settingsPath, readErr)
	}

	hooks := ensureObject(root, "hooks")
	removeHookEventCommandsContaining(hooks, "Stop", legacyScriptPath)
	removeHookEventCommandsContaining(hooks, "SessionEnd", legacyScriptPath)
	ensureHookEventCommand(hooks, "Stop", command)
	ensureHookEventCommand(hooks, "SessionEnd", command)

	permissions := ensureObject(root, "permissions")
	allowList := ensureAnyArray(permissions, "allow")
	requiredPerms := []string{
		fmt.Sprintf("Bash(%s:*)", scriptPath),
		fmt.Sprintf("Bash(%s:*)", command),
		fmt.Sprintf("Bash(%s hook complete:*)", convergeBin),
		"Bash(converge hook complete:*)",
	}
	for _, perm := range requiredPerms {
		if !arrayContainsString(allowList, perm) {
			allowList = append(allowList, perm)
		}
	}
	permissions["allow"] = allowList

	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal claude settings: %w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.WriteFile(settingsPath, encoded, 0o644); err != nil {
		return fmt.Errorf("write claude settings %s: %w", settingsPath, err)
	}

	fmt.Fprintf(out, "Installed Claude hooks in %s\n", settingsPath)
	return nil
}

func ensureObject(parent map[string]any, key string) map[string]any {
	if value, ok := parent[key]; ok {
		if obj, ok := value.(map[string]any); ok {
			return obj
		}
	}
	obj := map[string]any{}
	parent[key] = obj
	return obj
}

func ensureAnyArray(parent map[string]any, key string) []any {
	if value, ok := parent[key]; ok {
		if arr, ok := value.([]any); ok {
			return arr
		}
	}
	arr := []any{}
	parent[key] = arr
	return arr
}

func ensureHookEventCommand(hooks map[string]any, eventName, command string) {
	events := ensureAnyArray(hooks, eventName)

	var entry map[string]any
	for _, item := range events {
		node, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if matcher, _ := node["matcher"].(string); strings.TrimSpace(matcher) == "*" {
			entry = node
			break
		}
	}

	if entry == nil {
		entry = map[string]any{
			"matcher": "*",
			"hooks":   []any{},
		}
		events = append(events, entry)
		hooks[eventName] = events
	}

	hookEntries := ensureAnyArray(entry, "hooks")
	for _, hookItem := range hookEntries {
		hookNode, ok := hookItem.(map[string]any)
		if !ok {
			continue
		}
		if hookType, _ := hookNode["type"].(string); hookType != "command" {
			continue
		}
		if existingCommand, _ := hookNode["command"].(string); strings.TrimSpace(existingCommand) == command {
			entry["hooks"] = hookEntries
			return
		}
	}

	hookEntries = append(hookEntries, map[string]any{
		"type":    "command",
		"command": command,
	})
	entry["hooks"] = hookEntries
}

func removeHookEventCommandsContaining(hooks map[string]any, eventName, needle string) {
	events := ensureAnyArray(hooks, eventName)
	for _, item := range events {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		hookEntries := ensureAnyArray(entry, "hooks")
		filtered := make([]any, 0, len(hookEntries))
		for _, hookItem := range hookEntries {
			hookNode, ok := hookItem.(map[string]any)
			if !ok {
				filtered = append(filtered, hookItem)
				continue
			}
			hookType, _ := hookNode["type"].(string)
			existingCommand, _ := hookNode["command"].(string)
			if hookType == "command" && strings.Contains(strings.TrimSpace(existingCommand), needle) {
				continue
			}
			filtered = append(filtered, hookItem)
		}
		entry["hooks"] = filtered
	}
}

func arrayContainsString(values []any, target string) bool {
	for _, value := range values {
		if text, ok := value.(string); ok && text == target {
			return true
		}
	}
	return false
}
