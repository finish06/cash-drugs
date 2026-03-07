package cache

import "testing"

func TestEscapeRegex_NoSpecialChars(t *testing.T) {
	input := "drugnames"
	got := escapeRegex(input)
	if got != "drugnames" {
		t.Errorf("expected 'drugnames', got '%s'", got)
	}
}

func TestEscapeRegex_Dot(t *testing.T) {
	got := escapeRegex("file.name")
	expected := `file\.name`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestEscapeRegex_AllSpecialChars(t *testing.T) {
	// The special set in escapeRegex is: \.+*?^${}()|[]
	input := `a\.+*?^${}()|[]z`
	got := escapeRegex(input)
	expected := `a\\\.\+\*\?\^\$\{\}\(\)\|\[\]z`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestEscapeRegex_EmptyString(t *testing.T) {
	got := escapeRegex("")
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

func TestEscapeRegex_ParensAndPipe(t *testing.T) {
	got := escapeRegex("(foo|bar)")
	expected := `\(foo\|bar\)`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestEscapeRegex_CacheKeyWithSpecialChars(t *testing.T) {
	// Simulates a cache key that contains regex-sensitive characters
	got := escapeRegex("spl-detail:SETID=abc.123+456")
	expected := `spl-detail:SETID=abc\.123\+456`
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}
