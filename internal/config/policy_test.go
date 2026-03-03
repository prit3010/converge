package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseByteSizeString(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{in: "10MB", want: 10_000_000},
		{in: "1GiB", want: 1_073_741_824},
		{in: "42", want: 42},
		{in: "5 kb", want: 5_000},
	}

	for _, tc := range tests {
		got, err := parseByteSizeString(tc.in)
		if err != nil {
			t.Fatalf("parse %q: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("parse %q = %d, want %d", tc.in, got, tc.want)
		}
	}
}

func TestIgnoreMatcherSupportsCommentsGlobNegationAndTrailingSlash(t *testing.T) {
	matcher, err := compileIgnoreMatcher([]string{
		"# comment",
		"",
		"node_modules/",
		".env*",
		"!important.env",
		"build/**",
	})
	if err != nil {
		t.Fatalf("compile matcher: %v", err)
	}

	if !matcher.Matches("node_modules/pkg/index.js", false) {
		t.Fatalf("expected node_modules path to match")
	}
	if !matcher.Matches(".env.local", false) {
		t.Fatalf("expected env glob to match")
	}
	if matcher.Matches("important.env", false) {
		t.Fatalf("expected negation to unignore important.env")
	}
	if !matcher.Matches("build/output/main.js", false) {
		t.Fatalf("expected recursive glob to match")
	}
}

func TestLoadRepoPolicyMergesConfigAndIgnoreFile(t *testing.T) {
	projectDir := t.TempDir()
	stateDir := filepath.Join(projectDir, StateDirName)
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}

	configBody := []byte(`
[snapshot]
ignore = ["tmp/**"]
max_file_size = "10MB"
binary_policy = "fail"

[eval]
tests = ["go test ./..."]
lint = ["golangci-lint run ./..."]
`)
	if err := os.WriteFile(filepath.Join(stateDir, ConfigFileName), configBody, 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	ignoreBody := []byte(`
# comment
.env*
`)
	if err := os.WriteFile(filepath.Join(projectDir, IgnoreFileName), ignoreBody, 0o644); err != nil {
		t.Fatalf("write .convergeignore: %v", err)
	}

	policy, err := LoadRepoPolicy(projectDir)
	if err != nil {
		t.Fatalf("load policy: %v", err)
	}

	if policy.Snapshot.BinaryPolicy != BinaryPolicyFail {
		t.Fatalf("binary policy = %q, want %q", policy.Snapshot.BinaryPolicy, BinaryPolicyFail)
	}
	if policy.Snapshot.MaxFileSizeBytes != 10_000_000 {
		t.Fatalf("max_file_size = %d, want %d", policy.Snapshot.MaxFileSizeBytes, 10_000_000)
	}
	if !policy.ShouldIgnore(".env.production", false) {
		t.Fatalf("expected .env pattern from .convergeignore to apply")
	}
	if !policy.ShouldIgnore("tmp/cache/file.bin", false) {
		t.Fatalf("expected snapshot.ignore glob to apply")
	}
	if !policy.Eval.HasOverrides() {
		t.Fatalf("expected eval overrides from config")
	}
}
