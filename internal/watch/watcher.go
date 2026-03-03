package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
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

type IgnoreFunc func(relPath string, isDir bool) bool

func Watch(ctx context.Context, projectDir string, debounce time.Duration, ignore IgnoreFunc, onChange OnChange) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("create watcher: %w", err)
	}
	defer watcher.Close()

	if err := addDirsRecursive(watcher, projectDir, ignore); err != nil {
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
			if isIgnoredPath(projectDir, event.Name, false, ignore) {
				continue
			}
			if event.Has(fsnotify.Create) {
				if info, err := os.Stat(event.Name); err == nil && info.IsDir() {
					if !isIgnoredPath(projectDir, event.Name, true, ignore) {
						_ = addDirsRecursive(watcher, event.Name, ignore)
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

func addDirsRecursive(watcher *fsnotify.Watcher, root string, ignore IgnoreFunc) error {
	return filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root && isIgnoredPath(root, path, true, ignore) {
			return filepath.SkipDir
		}
		if err := watcher.Add(path); err != nil {
			return fmt.Errorf("watch add %s: %w", path, err)
		}
		return nil
	})
}

func isIgnoredPath(projectDir, path string, isDir bool, ignore IgnoreFunc) bool {
	if ignore == nil {
		return false
	}
	rel, err := filepath.Rel(projectDir, path)
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(filepath.Clean(rel))
	if rel == "." || rel == "" {
		return false
	}
	return ignore(rel, isDir)
}
