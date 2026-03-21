package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// validConfigYAML is a minimal valid config for testing.
const validConfigYAML = `endpoints:
  - slug: test-ep
    base_url: http://example.com
    path: /api/test
    format: json
`

// validConfigYAML2 has two endpoints to verify reload detects changes.
const validConfigYAML2 = `endpoints:
  - slug: test-ep
    base_url: http://example.com
    path: /api/test
    format: json
  - slug: test-ep2
    base_url: http://example.com
    path: /api/test2
    format: json
`

const invalidConfigYAML = `endpoints: [[[invalid`

func writeTempConfig(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestWatcher_FileChangeTriggersCallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, validConfigYAML)

	var mu sync.Mutex
	var received []Endpoint

	w := NewWatcher(cfgPath, func(eps []Endpoint) error {
		mu.Lock()
		received = eps
		mu.Unlock()
		return nil
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	// Wait for watcher to be ready
	time.Sleep(100 * time.Millisecond)

	// Modify config file — add a second endpoint
	if err := os.WriteFile(cfgPath, []byte(validConfigYAML2), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce + processing
	deadline := time.After(3 * time.Second)
	for {
		mu.Lock()
		got := len(received)
		mu.Unlock()
		if got == 2 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for reload callback; got %d endpoints", got)
		default:
			time.Sleep(50 * time.Millisecond)
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if received[0].Slug != "test-ep" {
		t.Errorf("expected first slug 'test-ep', got %q", received[0].Slug)
	}
	if received[1].Slug != "test-ep2" {
		t.Errorf("expected second slug 'test-ep2', got %q", received[1].Slug)
	}
}

func TestWatcher_InvalidYAMLDoesNotCrash(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, validConfigYAML)

	callCount := 0
	var mu sync.Mutex

	w := NewWatcher(cfgPath, func(eps []Endpoint) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Write invalid YAML
	if err := os.WriteFile(cfgPath, []byte(invalidConfigYAML), 0644); err != nil {
		t.Fatal(err)
	}

	// Wait for debounce
	time.Sleep(800 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if callCount != 0 {
		t.Errorf("callback should not fire on invalid config, fired %d times", callCount)
	}
}

func TestWatcher_DebounceRapidChanges(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, validConfigYAML)

	callCount := 0
	var mu sync.Mutex

	w := NewWatcher(cfgPath, func(eps []Endpoint) error {
		mu.Lock()
		callCount++
		mu.Unlock()
		return nil
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}
	defer w.Stop()

	time.Sleep(100 * time.Millisecond)

	// Write rapidly 5 times within debounce window
	for i := 0; i < 5; i++ {
		if err := os.WriteFile(cfgPath, []byte(validConfigYAML), 0644); err != nil {
			t.Fatal(err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Wait for debounce to settle + some margin
	time.Sleep(1 * time.Second)

	mu.Lock()
	defer mu.Unlock()
	// Should have been called once (debounced), not 5 times
	if callCount < 1 {
		t.Error("expected at least 1 callback after rapid changes")
	}
	if callCount > 2 {
		t.Errorf("expected debounce to coalesce rapid changes, got %d callbacks", callCount)
	}
}

func TestWatcher_StopIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	cfgPath := writeTempConfig(t, dir, validConfigYAML)

	w := NewWatcher(cfgPath, func(eps []Endpoint) error {
		return nil
	})

	if err := w.Start(); err != nil {
		t.Fatal(err)
	}

	// Stop should be safe to call multiple times
	w.Stop()
	w.Stop()
	w.Stop()
}
