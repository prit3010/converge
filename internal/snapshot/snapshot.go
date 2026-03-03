package snapshot

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/prit3010/converge/internal/config"
	"github.com/prit3010/converge/internal/store"
)

type FileEntry struct {
	Hash string
	Mode fs.FileMode
	Size int64
}

type Manifest map[string]FileEntry

type SkipReason struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type Snapshot struct {
	store       *store.Store
	policy      config.Policy
	lastSkipped []SkipReason
}

func New(s *store.Store) *Snapshot {
	snap := &Snapshot{store: s}
	snap.SetPolicy(config.DefaultPolicy())
	return snap
}

func NewWithPolicy(s *store.Store, policy config.Policy) *Snapshot {
	snap := New(s)
	snap.SetPolicy(policy)
	return snap
}

func (s *Snapshot) SetPolicy(policy config.Policy) {
	s.policy = policy
}

func (s *Snapshot) LastSkipped() []SkipReason {
	out := make([]SkipReason, len(s.lastSkipped))
	copy(out, s.lastSkipped)
	return out
}

func (s *Snapshot) Capture(projectDir string) (Manifest, error) {
	manifest := make(Manifest)
	skipped := make([]SkipReason, 0)
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath := ""
		if path != projectDir {
			rel, relErr := filepath.Rel(projectDir, path)
			if relErr != nil {
				return fmt.Errorf("relative path for %s: %w", path, relErr)
			}
			relPath = filepath.ToSlash(rel)
		}

		if d.IsDir() {
			if relPath != "" && s.policy.ShouldIgnore(relPath, true) {
				return filepath.SkipDir
			}
			return nil
		}

		if relPath == "" {
			return nil
		}
		if s.policy.ShouldIgnore(relPath, false) {
			skipped = append(skipped, SkipReason{Path: relPath, Reason: "ignored_by_policy"})
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", relPath, err)
		}
		if s.policy.Snapshot.MaxFileSizeBytes > 0 && info.Size() > s.policy.Snapshot.MaxFileSizeBytes {
			skipped = append(skipped, SkipReason{Path: relPath, Reason: "max_file_size_exceeded"})
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", relPath, err)
		}
		if !IsText(data) {
			switch s.policy.Snapshot.BinaryPolicy {
			case config.BinaryPolicySkip:
				skipped = append(skipped, SkipReason{Path: relPath, Reason: "binary_skipped"})
				return nil
			case config.BinaryPolicyFail:
				return fmt.Errorf("snapshot policy blocks binary file %s", relPath)
			}
		}

		hash, err := s.store.Write(data)
		if err != nil {
			return fmt.Errorf("store %s: %w", relPath, err)
		}
		manifest[relPath] = FileEntry{Hash: hash, Mode: info.Mode(), Size: info.Size()}
		return nil
	})
	if err != nil {
		s.lastSkipped = skipped
		return nil, fmt.Errorf("walk project: %w", err)
	}
	s.lastSkipped = skipped
	return manifest, nil
}

// CapturePaths snapshots only the provided repository-relative file paths.
// Paths that resolve outside the project directory are rejected.
func (s *Snapshot) CapturePaths(projectDir string, paths []string) (Manifest, error) {
	manifest := make(Manifest)
	skipped := make([]SkipReason, 0)
	seen := make(map[string]struct{}, len(paths))

	for _, raw := range paths {
		normalized := filepath.ToSlash(strings.TrimSpace(raw))
		if normalized == "" || normalized == "." {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}

		cleaned := filepath.Clean(normalized)
		cleaned = filepath.ToSlash(cleaned)
		if cleaned == "." {
			continue
		}
		if strings.HasPrefix(cleaned, "../") || strings.HasPrefix(cleaned, "/") {
			return nil, fmt.Errorf("path escapes project: %s", normalized)
		}
		if s.policy.ShouldIgnore(cleaned, false) {
			skipped = append(skipped, SkipReason{Path: cleaned, Reason: "ignored_by_policy"})
			continue
		}

		fullPath := filepath.Join(projectDir, filepath.FromSlash(cleaned))
		info, err := os.Lstat(fullPath)
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", cleaned, err)
		}
		if info.IsDir() {
			continue
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			continue
		}
		if s.policy.Snapshot.MaxFileSizeBytes > 0 && info.Size() > s.policy.Snapshot.MaxFileSizeBytes {
			skipped = append(skipped, SkipReason{Path: cleaned, Reason: "max_file_size_exceeded"})
			continue
		}

		data, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", cleaned, err)
		}
		if !IsText(data) {
			switch s.policy.Snapshot.BinaryPolicy {
			case config.BinaryPolicySkip:
				skipped = append(skipped, SkipReason{Path: cleaned, Reason: "binary_skipped"})
				continue
			case config.BinaryPolicyFail:
				return nil, fmt.Errorf("snapshot policy blocks binary file %s", cleaned)
			}
		}
		hash, err := s.store.Write(data)
		if err != nil {
			return nil, fmt.Errorf("store %s: %w", cleaned, err)
		}
		manifest[cleaned] = FileEntry{Hash: hash, Mode: info.Mode(), Size: info.Size()}
	}

	s.lastSkipped = skipped
	return manifest, nil
}

func EqualToEntries(m Manifest, entries map[string]string) bool {
	if len(m) != len(entries) {
		return false
	}
	for path, entry := range m {
		if entries[path] != entry.Hash {
			return false
		}
	}
	return true
}

func SortedPaths(m Manifest) []string {
	paths := make([]string, 0, len(m))
	for path := range m {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func IsText(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return true
}
