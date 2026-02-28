package eval

import (
	"path/filepath"
	"strings"
)

// CountLOC counts non-empty, non-comment lines for known file types.
func CountLOC(filename string, content string) int {
	if content == "" {
		return 0
	}
	ext := strings.ToLower(filepath.Ext(filename))
	count := 0
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if isComment(ext, trimmed) {
			continue
		}
		count++
	}
	return count
}

func isComment(ext string, line string) bool {
	switch ext {
	case ".go", ".js", ".ts", ".tsx", ".jsx", ".java", ".c", ".cpp", ".rs", ".swift":
		return strings.HasPrefix(line, "//")
	case ".py", ".rb", ".sh", ".yaml", ".yml", ".toml":
		return strings.HasPrefix(line, "#")
	case ".sql", ".lua":
		return strings.HasPrefix(line, "--")
	default:
		return false
	}
}
