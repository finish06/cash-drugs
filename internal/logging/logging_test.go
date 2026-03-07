package logging_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"github.com/finish06/drugs/internal/logging"
)

// AC-002: Default log level is warn when no configuration is provided
func TestAC002_DefaultLogLevelIsWarn(t *testing.T) {
	level, err := logging.ParseLevel("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if level != slog.LevelWarn {
		t.Errorf("expected default level Warn, got %v", level)
	}
}

// AC-003: LOG_LEVEL env var sets the log level
func TestAC003_ParseLevelValid(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"DEBUG", slog.LevelDebug},
		{"Info", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"ERROR", slog.LevelError},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			level, err := logging.ParseLevel(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if level != tc.expected {
				t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, level, tc.expected)
			}
		})
	}
}

// AC-006: Invalid log level values are rejected with clear error
func TestAC006_InvalidLogLevelRejected(t *testing.T) {
	invalids := []string{"verbose", "trace", "critical", "warning", "off", "123"}
	for _, input := range invalids {
		t.Run(input, func(t *testing.T) {
			_, err := logging.ParseLevel(input)
			if err == nil {
				t.Errorf("expected error for invalid level %q, got nil", input)
			}
			if !strings.Contains(err.Error(), "must be one of") {
				t.Errorf("error should mention valid values, got: %v", err)
			}
		})
	}
}

// AC-007: Default output format is JSON
func TestAC007_DefaultFormatIsJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelInfo, "", &buf)

	logger.Info("test message")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("expected JSON output, got parse error: %v\nOutput: %s", err, buf.String())
	}
	if _, ok := logEntry["msg"]; !ok {
		t.Error("expected JSON log entry to contain 'msg' field")
	}
}

// AC-008: LOG_FORMAT=text switches to plain text output
func TestAC008_TextFormatOutput(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelInfo, "text", &buf)

	logger.Info("test message")

	output := buf.String()
	// Text format should NOT be valid JSON
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(output), &logEntry); err == nil {
		t.Error("expected non-JSON output for text format, but got valid JSON")
	}
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain 'test message', got: %s", output)
	}
}

// AC-007/AC-008: JSON format explicitly requested
func TestAC007_JSONFormatExplicit(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelInfo, "json", &buf)

	logger.Info("hello")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("expected JSON output, got: %s", buf.String())
	}
}

// AC-002: Setup with warn level filters info messages
func TestAC002_WarnLevelFiltersInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelWarn, "json", &buf)

	logger.Info("should not appear")
	logger.Debug("should not appear either")

	if buf.Len() != 0 {
		t.Errorf("expected no output at warn level for info/debug messages, got: %s", buf.String())
	}
}

// AC-002: Setup with warn level allows warn and error
func TestAC002_WarnLevelAllowsWarnAndError(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelWarn, "json", &buf)

	logger.Warn("warning message")
	logger.Error("error message")

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 log lines, got %d: %s", len(lines), buf.String())
	}
}

// AC-013: Log entries include component field
func TestAC013_ComponentField(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelInfo, "json", &buf)

	componentLogger := logger.With("component", "handler")
	componentLogger.Info("test")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}
	if logEntry["component"] != "handler" {
		t.Errorf("expected component=handler, got %v", logEntry["component"])
	}
}

// Edge case: empty LOG_FORMAT defaults to JSON
func TestEdge_EmptyFormatDefaultsToJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelInfo, "", &buf)

	logger.Info("test")

	var logEntry map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("expected JSON for empty format, got: %s", buf.String())
	}
}

// Edge case: invalid LOG_FORMAT defaults to JSON
func TestEdge_InvalidFormatDefaultsToJSON(t *testing.T) {
	var buf bytes.Buffer
	logger := logging.Setup(slog.LevelInfo, "yaml", &buf)

	logger.Info("test")

	output := buf.String()
	// Should default to JSON despite invalid format, but first line might be a warning
	lines := strings.Split(strings.TrimSpace(output), "\n")
	// Last log line should be valid JSON
	lastLine := lines[len(lines)-1]
	var logEntry map[string]interface{}
	if err := json.Unmarshal([]byte(lastLine), &logEntry); err != nil {
		t.Fatalf("expected JSON fallback for invalid format, got: %s", output)
	}
}
