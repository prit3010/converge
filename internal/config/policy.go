package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
)

type BinaryPolicy string

const (
	BinaryPolicySkip    BinaryPolicy = "skip"
	BinaryPolicyInclude BinaryPolicy = "include"
	BinaryPolicyFail    BinaryPolicy = "fail"
)

type SnapshotPolicy struct {
	IgnorePatterns   []string
	MaxFileSizeBytes int64
	BinaryPolicy     BinaryPolicy
}

type EvalPolicy struct {
	Tests []string
	Lint  []string
	Types []string
}

func (e EvalPolicy) HasOverrides() bool {
	return len(e.Tests) > 0 || len(e.Lint) > 0 || len(e.Types) > 0
}

type Policy struct {
	Snapshot SnapshotPolicy
	Eval     EvalPolicy

	ignoreMatcher *IgnoreMatcher
}

type rawConfig struct {
	Snapshot rawSnapshot `toml:"snapshot"`
	Eval     rawEval     `toml:"eval"`
}

type rawSnapshot struct {
	Ignore       []string `toml:"ignore"`
	MaxFileSize  any      `toml:"max_file_size"`
	BinaryPolicy string   `toml:"binary_policy"`
}

type rawEval struct {
	Tests []string `toml:"tests"`
	Lint  []string `toml:"lint"`
	Types []string `toml:"types"`
}

func DefaultPolicy() Policy {
	policy := Policy{
		Snapshot: SnapshotPolicy{
			IgnorePatterns:   append([]string(nil), BuiltinIgnorePatterns...),
			MaxFileSizeBytes: 0,
			BinaryPolicy:     BinaryPolicySkip,
		},
		Eval: EvalPolicy{},
	}
	matcher, _ := compileIgnoreMatcher(policy.Snapshot.IgnorePatterns)
	policy.ignoreMatcher = matcher
	return policy
}

func LoadRepoPolicy(projectDir string) (Policy, error) {
	policy := DefaultPolicy()

	raw, err := readConfig(projectDir)
	if err != nil {
		return Policy{}, err
	}
	if err := applyRawConfig(&policy, raw); err != nil {
		return Policy{}, err
	}

	ignorePath := filepath.Join(projectDir, IgnoreFileName)
	if lines, err := readIgnorePatterns(ignorePath); err == nil {
		policy.Snapshot.IgnorePatterns = append(policy.Snapshot.IgnorePatterns, lines...)
	} else if !os.IsNotExist(err) {
		return Policy{}, fmt.Errorf("read %s: %w", ignorePath, err)
	}

	matcher, err := compileIgnoreMatcher(policy.Snapshot.IgnorePatterns)
	if err != nil {
		return Policy{}, err
	}
	policy.ignoreMatcher = matcher
	return policy, nil
}

func (p Policy) ShouldIgnore(relPath string, isDir bool) bool {
	normalized := normalizeRelPath(relPath)
	if normalized == "" {
		return false
	}
	if normalized == StateDirName || strings.HasPrefix(normalized, StateDirName+"/") {
		return true
	}
	return p.ignoreMatcher != nil && p.ignoreMatcher.Matches(normalized, isDir)
}

func readConfig(projectDir string) (*rawConfig, error) {
	path := filepath.Join(projectDir, StateDirName, ConfigFileName)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &rawConfig{}, nil
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var cfg rawConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	return &cfg, nil
}

func applyRawConfig(policy *Policy, raw *rawConfig) error {
	if policy == nil || raw == nil {
		return nil
	}

	if raw.Snapshot.BinaryPolicy != "" {
		bp := BinaryPolicy(strings.TrimSpace(strings.ToLower(raw.Snapshot.BinaryPolicy)))
		if bp != BinaryPolicySkip && bp != BinaryPolicyInclude && bp != BinaryPolicyFail {
			return fmt.Errorf("invalid snapshot.binary_policy %q (expected skip|include|fail)", raw.Snapshot.BinaryPolicy)
		}
		policy.Snapshot.BinaryPolicy = bp
	}

	if raw.Snapshot.MaxFileSize != nil {
		sizeBytes, err := parseByteSize(raw.Snapshot.MaxFileSize)
		if err != nil {
			return fmt.Errorf("invalid snapshot.max_file_size: %w", err)
		}
		policy.Snapshot.MaxFileSizeBytes = sizeBytes
	}

	if len(raw.Snapshot.Ignore) > 0 {
		policy.Snapshot.IgnorePatterns = append(policy.Snapshot.IgnorePatterns, raw.Snapshot.Ignore...)
	}

	policy.Eval = EvalPolicy{
		Tests: normalizeCommandList(raw.Eval.Tests),
		Lint:  normalizeCommandList(raw.Eval.Lint),
		Types: normalizeCommandList(raw.Eval.Types),
	}
	return nil
}

func normalizeCommandList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func parseByteSize(value any) (int64, error) {
	switch v := value.(type) {
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("must be >= 0")
		}
		return v, nil
	case int32:
		if v < 0 {
			return 0, fmt.Errorf("must be >= 0")
		}
		return int64(v), nil
	case int:
		if v < 0 {
			return 0, fmt.Errorf("must be >= 0")
		}
		return int64(v), nil
	case float64:
		if v < 0 {
			return 0, fmt.Errorf("must be >= 0")
		}
		return int64(v), nil
	case string:
		return parseByteSizeString(v)
	default:
		return 0, fmt.Errorf("unsupported type %T", value)
	}
}

func parseByteSizeString(raw string) (int64, error) {
	text := strings.TrimSpace(strings.ToLower(raw))
	if text == "" {
		return 0, fmt.Errorf("cannot be empty")
	}
	var numberPart strings.Builder
	var unitPart strings.Builder
	for _, r := range text {
		if r >= '0' && r <= '9' {
			if unitPart.Len() > 0 {
				return 0, fmt.Errorf("invalid size %q", raw)
			}
			numberPart.WriteRune(r)
			continue
		}
		if r == ' ' || r == '\t' {
			continue
		}
		unitPart.WriteRune(r)
	}
	if numberPart.Len() == 0 {
		return 0, fmt.Errorf("invalid size %q", raw)
	}

	var value int64
	if _, err := fmt.Sscanf(numberPart.String(), "%d", &value); err != nil {
		return 0, fmt.Errorf("parse number: %w", err)
	}
	if value < 0 {
		return 0, fmt.Errorf("must be >= 0")
	}

	multiplier := int64(1)
	switch unitPart.String() {
	case "", "b":
		multiplier = 1
	case "k", "kb":
		multiplier = 1000
	case "m", "mb":
		multiplier = 1000 * 1000
	case "g", "gb":
		multiplier = 1000 * 1000 * 1000
	case "kib":
		multiplier = 1024
	case "mib":
		multiplier = 1024 * 1024
	case "gib":
		multiplier = 1024 * 1024 * 1024
	default:
		return 0, fmt.Errorf("unsupported unit %q", unitPart.String())
	}
	return value * multiplier, nil
}
