package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReadAndDedup(t *testing.T) {
	tmp := t.TempDir()
	s := New(tmp)

	h1, err := s.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write 1: %v", err)
	}
	h2, err := s.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("write 2: %v", err)
	}
	if h1 != h2 {
		t.Fatalf("expected same hash, got %s and %s", h1, h2)
	}

	data, err := s.Read(h1)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected payload: %q", string(data))
	}

	files := 0
	err = filepath.WalkDir(tmp, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			files++
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk objects: %v", err)
	}
	if files != 1 {
		t.Fatalf("expected 1 stored blob, got %d", files)
	}
}
