package watcher

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Event represents a file change event.
type Event struct {
	// Size is the new file size.
	Size int64
	// Truncated is true if the file was truncated (size decreased).
	Truncated bool
}

// Watcher watches a file for changes using polling.
type Watcher interface {
	// Watch starts watching the file and sends events on the returned channel.
	// The channel is closed when the context is cancelled or an error occurs.
	// Returns an error if the file cannot be accessed initially.
	Watch(ctx context.Context) (<-chan Event, error)
}

// Config holds watcher configuration.
type Config struct {
	// Path is the file to watch.
	Path string
	// PollInterval is how often to check for changes.
	PollInterval time.Duration
}

// pollingWatcher implements Watcher using polling.
type pollingWatcher struct {
	config Config
}

// NewWatcher creates a new polling-based file watcher.
func NewWatcher(config Config) Watcher {
	return &pollingWatcher{config: config}
}

// Watch starts watching the file and sends events on the returned channel.
func (w *pollingWatcher) Watch(ctx context.Context) (<-chan Event, error) {
	// Check file exists initially
	info, err := os.Stat(w.config.Path)
	if err != nil {
		return nil, fmt.Errorf("accessing %s: %w", w.config.Path, err)
	}

	events := make(chan Event)
	lastSize := info.Size()

	go func() {
		defer close(events)

		ticker := time.NewTicker(w.config.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				info, err := os.Stat(w.config.Path)
				if err != nil {
					// File might be temporarily unavailable during rotation
					continue
				}

				currentSize := info.Size()
				if currentSize == lastSize {
					continue
				}

				evt := Event{Size: currentSize}
				if currentSize < lastSize {
					evt.Truncated = true
				}

				select {
				case events <- evt:
					lastSize = currentSize
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return events, nil
}
