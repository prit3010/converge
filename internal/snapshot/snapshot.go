package snapshot

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/prittamravi/converge/internal/config"
	"github.com/prittamravi/converge/internal/store"
)

type FileEntry struct {
	Hash string
	Mode fs.FileMode
	Size int64
}

type Manifest map[string]FileEntry

type Snapshot struct {
	store *store.Store
}

func New(s *store.Store) *Snapshot {
	return &Snapshot{store: s}
}

func (s *Snapshot) Capture(projectDir string) (Manifest, error) {
	manifest := make(Manifest)
	err := filepath.WalkDir(projectDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()

		if d.IsDir() {
			if path != projectDir {
				if _, ignored := config.IgnoredDirNames[name]; ignored {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if _, ignored := config.IgnoredFileNames[name]; ignored {
			return nil
		}
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		relPath, err := filepath.Rel(projectDir, path)
		if err != nil {
			return fmt.Errorf("relative path for %s: %w", path, err)
		}
		relPath = filepath.ToSlash(relPath)

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", relPath, err)
		}
		hash, err := s.store.Write(data)
		if err != nil {
			return fmt.Errorf("store %s: %w", relPath, err)
		}
		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("stat %s: %w", relPath, err)
		}
		manifest[relPath] = FileEntry{Hash: hash, Mode: info.Mode(), Size: info.Size()}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk project: %w", err)
	}
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
