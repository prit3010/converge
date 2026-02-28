package store

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// Store is a content-addressed object store of raw file blobs.
type Store struct {
	root string
}

func New(root string) *Store {
	return &Store{root: root}
}

func (s *Store) blobPath(hash string) string {
	if len(hash) < 2 {
		return filepath.Join(s.root, hash)
	}
	return filepath.Join(s.root, hash[:2], hash)
}

func (s *Store) Write(data []byte) (string, error) {
	digest := sha256.Sum256(data)
	hash := hex.EncodeToString(digest[:])
	if s.Has(hash) {
		return hash, nil
	}
	path := s.blobPath(hash)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("create object dir: %w", err)
	}
	if err := os.WriteFile(path, data, 0o444); err != nil {
		return "", fmt.Errorf("write object %s: %w", hash, err)
	}
	return hash, nil
}

func (s *Store) Read(hash string) ([]byte, error) {
	data, err := os.ReadFile(s.blobPath(hash))
	if err != nil {
		return nil, fmt.Errorf("read object %s: %w", hash, err)
	}
	return data, nil
}

func (s *Store) Has(hash string) bool {
	_, err := os.Stat(s.blobPath(hash))
	return err == nil
}
