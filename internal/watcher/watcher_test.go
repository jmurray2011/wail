package watcher

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatcher_DetectsGrowth(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create initial file
	if err := os.WriteFile(testFile, []byte("line1\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	w := NewWatcher(Config{
		Path:         testFile,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	events, err := w.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Append to file
	time.Sleep(20 * time.Millisecond)
	f, err := os.OpenFile(testFile, os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("failed to open file for append: %v", err)
	}
	f.WriteString("line2\n")
	f.Close()

	// Wait for event
	select {
	case evt, ok := <-events:
		if !ok {
			t.Fatal("channel closed without event")
		}
		if evt.Truncated {
			t.Error("expected Truncated=false for growth")
		}
		if evt.Size <= 6 { // "line1\n" = 6 bytes
			t.Errorf("expected Size > 6, got %d", evt.Size)
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for growth event")
	}
}

func TestWatcher_DetectsTruncation(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	// Create file with content
	if err := os.WriteFile(testFile, []byte("line1\nline2\nline3\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	w := NewWatcher(Config{
		Path:         testFile,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	events, err := w.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Truncate file (simulates log rotation)
	time.Sleep(20 * time.Millisecond)
	if err := os.WriteFile(testFile, []byte("new\n"), 0644); err != nil {
		t.Fatalf("failed to truncate file: %v", err)
	}

	// Wait for truncation event
	select {
	case evt, ok := <-events:
		if !ok {
			t.Fatal("channel closed without event")
		}
		if !evt.Truncated {
			t.Error("expected Truncated=true for truncation")
		}
	case <-ctx.Done():
		t.Fatal("timeout waiting for truncation event")
	}
}

func TestWatcher_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	testFile := filepath.Join(dir, "test.log")

	if err := os.WriteFile(testFile, []byte("content\n"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	w := NewWatcher(Config{
		Path:         testFile,
		PollInterval: 10 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())

	events, err := w.Watch(ctx)
	if err != nil {
		t.Fatalf("Watch() error = %v", err)
	}

	// Cancel context
	cancel()

	// Channel should close
	select {
	case _, ok := <-events:
		if ok {
			// Got an event, drain and check for close
			select {
			case _, ok := <-events:
				if ok {
					t.Error("expected channel to close after context cancellation")
				}
			case <-time.After(100 * time.Millisecond):
				t.Error("timeout waiting for channel close")
			}
		}
		// Channel closed as expected
	case <-time.After(100 * time.Millisecond):
		t.Error("timeout waiting for channel close after cancellation")
	}
}

func TestWatcher_NonExistentFile(t *testing.T) {
	w := NewWatcher(Config{
		Path:         "/nonexistent/file.log",
		PollInterval: 10 * time.Millisecond,
	})

	ctx := context.Background()
	_, err := w.Watch(ctx)
	if err == nil {
		t.Error("expected error for non-existent file")
	}
}
