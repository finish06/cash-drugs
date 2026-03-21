package config

import (
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// Watcher monitors a config file for changes and triggers a reload callback.
// It debounces rapid changes and also listens for SIGHUP to trigger manual reloads.
type Watcher struct {
	path     string
	onChange func([]Endpoint) error
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
}

// NewWatcher creates a config file watcher that calls onChange when the config
// file is modified or a SIGHUP signal is received. The callback receives the
// newly validated endpoints. Invalid config files are logged and skipped.
func NewWatcher(path string, onChange func([]Endpoint) error) *Watcher {
	return &Watcher{
		path:     path,
		onChange: onChange,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
	}
}

// Start begins watching the config file for changes and listening for SIGHUP.
// It returns an error if the fsnotify watcher cannot be created.
func (w *Watcher) Start() error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	if err := fsw.Add(w.path); err != nil {
		fsw.Close()
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGHUP)

	go w.loop(fsw, sigCh)

	slog.Info("config watcher started", "component", "config", "path", w.path)
	return nil
}

// Stop stops the watcher goroutine and cleans up resources.
func (w *Watcher) Stop() {
	w.once.Do(func() {
		close(w.stop)
		<-w.done
	})
}

func (w *Watcher) loop(fsw *fsnotify.Watcher, sigCh chan os.Signal) {
	defer close(w.done)
	defer fsw.Close()
	defer signal.Stop(sigCh)

	var debounceTimer *time.Timer
	var debounceCh <-chan time.Time

	const debounceDelay = 500 * time.Millisecond

	for {
		select {
		case <-w.stop:
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			return

		case event, ok := <-fsw.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Write|fsnotify.Create) == 0 {
				continue
			}
			slog.Debug("config file change detected", "component", "config", "op", event.Op.String())
			// Debounce: reset timer on each event
			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.NewTimer(debounceDelay)
			debounceCh = debounceTimer.C

		case err, ok := <-fsw.Errors:
			if !ok {
				return
			}
			slog.Error("config watcher error", "component", "config", "error", err)

		case <-debounceCh:
			debounceCh = nil
			debounceTimer = nil
			w.reload()

		case <-sigCh:
			slog.Info("SIGHUP received, reloading config", "component", "config")
			// Cancel any pending debounce
			if debounceTimer != nil {
				debounceTimer.Stop()
				debounceCh = nil
				debounceTimer = nil
			}
			w.reload()
		}
	}
}

func (w *Watcher) reload() {
	endpoints, err := Load(w.path)
	if err != nil {
		slog.Error("config reload failed — keeping current config", "component", "config", "error", err)
		return
	}

	if err := w.onChange(endpoints); err != nil {
		slog.Error("config reload callback failed", "component", "config", "error", err)
		return
	}

	slog.Info("config reloaded successfully", "component", "config", "endpoints", len(endpoints))
}
