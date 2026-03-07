package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// ParseLevel converts a string log level to slog.Level.
// Returns slog.LevelWarn for empty string (default).
func ParseLevel(s string) (slog.Level, error) {
	if s == "" {
		return slog.LevelWarn, nil
	}

	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("invalid log level %q: must be one of debug, info, warn, error", s)
	}
}

// Setup creates a configured slog.Logger writing to the given writer.
// If w is nil, writes to os.Stderr.
// Format can be "json" (default) or "text".
func Setup(level slog.Level, format string, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}

	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	switch strings.ToLower(format) {
	case "text":
		handler = slog.NewTextHandler(w, opts)
	default:
		handler = slog.NewJSONHandler(w, opts)
	}

	return slog.New(handler)
}
