package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

type ignoreRule struct {
	pattern  string
	negate   bool
	dirOnly  bool
	anchored bool
	hasSlash bool
}

type IgnoreMatcher struct {
	rules []ignoreRule
}

func compileIgnoreMatcher(patterns []string) (*IgnoreMatcher, error) {
	rules := make([]ignoreRule, 0, len(patterns))
	for _, raw := range patterns {
		rule, ok, err := parseIgnoreRule(raw)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}
	return &IgnoreMatcher{rules: rules}, nil
}

func readIgnorePatterns(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lines := make([]string, 0, 32)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan %s: %w", path, err)
	}
	return lines, nil
}

func parseIgnoreRule(raw string) (ignoreRule, bool, error) {
	line := strings.TrimSpace(raw)
	if line == "" {
		return ignoreRule{}, false, nil
	}
	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	}
	if strings.HasPrefix(line, "#") {
		return ignoreRule{}, false, nil
	}

	negate := false
	if strings.HasPrefix(line, "!") {
		negate = true
		line = strings.TrimSpace(line[1:])
	}
	if line == "" {
		return ignoreRule{}, false, nil
	}

	dirOnly := strings.HasSuffix(line, "/")
	line = strings.TrimSuffix(line, "/")

	anchored := strings.HasPrefix(line, "/")
	line = strings.TrimPrefix(line, "/")

	line = filepath.ToSlash(filepath.Clean(line))
	line = strings.TrimSpace(line)
	if line == "" || line == "." {
		return ignoreRule{}, false, nil
	}

	// Validate the glob pattern early to surface config mistakes at startup.
	if _, err := doublestar.PathMatch(line, line); err != nil {
		return ignoreRule{}, false, fmt.Errorf("invalid ignore pattern %q: %w", raw, err)
	}

	return ignoreRule{
		pattern:  line,
		negate:   negate,
		dirOnly:  dirOnly,
		anchored: anchored,
		hasSlash: strings.Contains(line, "/"),
	}, true, nil
}

func (m *IgnoreMatcher) Matches(relPath string, isDir bool) bool {
	if m == nil {
		return false
	}
	path := normalizeRelPath(relPath)
	if path == "" {
		return false
	}

	ignored := false
	for _, rule := range m.rules {
		if rule.matches(path, isDir) {
			ignored = !rule.negate
		}
	}
	return ignored
}

func (r ignoreRule) matches(path string, isDir bool) bool {
	if r.dirOnly {
		for _, dir := range pathDirs(path, isDir) {
			if r.matchNonDirOnly(dir) {
				return true
			}
		}
		return false
	}
	return r.matchNonDirOnly(path)
}

func (r ignoreRule) matchNonDirOnly(path string) bool {
	if !r.hasSlash {
		parts := strings.Split(path, "/")
		for _, part := range parts {
			if matchIgnoreGlob(r.pattern, part) {
				return true
			}
		}
		return false
	}
	if r.anchored {
		return matchIgnoreGlob(r.pattern, path)
	}
	return matchIgnoreGlob(r.pattern, path) || matchIgnoreGlob("**/"+r.pattern, path)
}

func matchIgnoreGlob(pattern, value string) bool {
	ok, err := doublestar.PathMatch(pattern, value)
	return err == nil && ok
}

func pathDirs(path string, isDir bool) []string {
	parts := strings.Split(path, "/")
	limit := len(parts) - 1
	if isDir {
		limit = len(parts)
	}
	if limit <= 0 {
		return nil
	}
	dirs := make([]string, 0, limit)
	for i := 1; i <= limit; i++ {
		dirs = append(dirs, strings.Join(parts[:i], "/"))
	}
	return dirs
}

func normalizeRelPath(path string) string {
	p := strings.TrimSpace(filepath.ToSlash(path))
	if p == "" || p == "." {
		return ""
	}
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	p = filepath.ToSlash(filepath.Clean(p))
	if p == "." {
		return ""
	}
	return p
}
