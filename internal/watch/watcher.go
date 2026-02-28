package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/prittamravi/converge/internal/config"
)

type Debouncer struct {
	mu       sync.Mutex
	timer    *time.Timer
	duration time.Duration
	fn       func()
}

func NewDebouncer(duration time.Duration, fn func()) *Debouncer {
	return &Debouncer{duration: duration, fn: fn}
}

func (d *Debouncer) Trigger() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.timer != nil {
		d.timer.Stop()
	}
	d.timer = time.AfterFunc(d.duration, d.fn)
}

type OnChange func() error

func Watch(ctx context.Context, projectDir string, debounce time.Duration, onChange OnChange) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	if err := addDirsRecursive(watcher, projectDir); err != nil {
		return err
	}

	debouncedChanges := make(chan struct{}, 1)
	debouncer := NewDebouncer(debounce, func() {
		select {
		case debouncedChanges <- struct{}{}:
		default:
		}
	})

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-debouncedChanges:
			if err := onChange(); err != nil {
				return fmt.Errorf("watch callback: %w", err)
			}
		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			if isIgnoredPath(projectDir, event.Name) {
				continue
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if !isIgnoredPath(projectDir, event.Name) {
						_ = addDirsRecursive(watcher, event.Name)
					}
				}
			}
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Remove) || event.Has(fsnotify.Rename) {
				debouncer.Trigger()
			}
		case watchErr, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			return fmt.Errorf("watcher error: %w", watchErr)
		}
	}
}

func addDirsRecursive(watcher *fsnotify.Watcher, root string) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isIgnoredPath(root, path) {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("watch add %s: %w", path, err)
		}
		return nil
	})
}

func isIgnoredPath(projectDir, path string) bool {
	rel, err := filepath.Rel(projectDir, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if rel == "." {
		return false
	}
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		if _, ignored := config.IgnoredDirNames[part]; ignored {
			return true
		}
	}
	return false
}
