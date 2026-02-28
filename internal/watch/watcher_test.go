package watch

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDebouncerCoalescesEvents(t *testing.T) {
	triggered := make(chan struct{}, 10)
	d := NewDebouncer(60*time.Millisecond, func() {
		triggered <- struct{}{}
	})
	for i := 0; i < 5; i++ {
		d.Trigger()
		time.Sleep(10 * time.Millisecond)
	}

	select {
	case <-triggered:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("debouncer did not fire")
	}

	select {
	case <-triggered:
		t.Fatalf("debouncer fired more than once")
	case <-time.After(150 * time.Millisecond):
	}
}

func TestIsIgnoredPath(t *testing.T) {
	root := "/tmp/project"
	if !isIgnoredPath(root, filepath.Join(root, ".converge", "db")) {
		t.Fatalf("expected .converge path to be ignored")
	}
	if isIgnoredPath(root, filepath.Join(root, "pkg", "main.go")) {
		t.Fatalf("did not expect pkg/main.go to be ignored")
	}
}

func TestWatchReturnsCallbackError(t *testing.T) {
	projectDir := t.TempDir()
	target := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	sentinel := errors.New("callback failed")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- Watch(ctx, projectDir, 25*time.Millisecond, func() error {
			return sentinel
		})
	}()

	time.Sleep(100 * time.Millisecond)
	if err := os.WriteFile(target, []byte("package main\n// changed\n"), 0o644); err != nil {
		t.Fatalf("rewrite target file: %v", err)
	}

	select {
	case err := <-errCh:
		if err == nil {
			t.Fatalf("expected callback error")
		}
		if !strings.Contains(err.Error(), "callback failed") {
			t.Fatalf("unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("watch did not return callback error")
	}
}

func TestWatchCallbacksDoNotOverlap(t *testing.T) {
	projectDir := t.TempDir()
	target := filepath.Join(projectDir, "main.go")
	if err := os.WriteFile(target, []byte("package main\n"), 0o644); err != nil {
		t.Fatalf("write target file: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var callbackCount int32
	var inflight int32
	var maxInflight int32
	errCh := make(chan error, 1)

	go func() {
		errCh <- Watch(ctx, projectDir, 20*time.Millisecond, func() error {
			current := atomic.AddInt32(&inflight, 1)
			for {
				previous := atomic.LoadInt32(&maxInflight)
				if current <= previous || atomic.CompareAndSwapInt32(&maxInflight, previous, current) {
					break
				}
			}
			time.Sleep(180 * time.Millisecond)
			atomic.AddInt32(&inflight, -1)
			if atomic.AddInt32(&callbackCount, 1) >= 2 {
				cancel()
			}
			return nil
		})
	}()

	time.Sleep(120 * time.Millisecond)
	if err := os.WriteFile(target, []byte("package main\n// first\n"), 0o644); err != nil {
		t.Fatalf("write first change: %v", err)
	}
	time.Sleep(80 * time.Millisecond)
	if err := os.WriteFile(target, []byte("package main\n// second\n"), 0o644); err != nil {
		t.Fatalf("write second change: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("watch returned unexpected error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("watch did not stop after callbacks")
	}

	if got := atomic.LoadInt32(&callbackCount); got < 2 {
		t.Fatalf("expected at least 2 callbacks, got %d", got)
	}
	if got := atomic.LoadInt32(&maxInflight); got > 1 {
		t.Fatalf("expected non-overlapping callbacks, max inflight=%d", got)
	}
}
