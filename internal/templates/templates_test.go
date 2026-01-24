// Package templates tests embedded template loading and validation.
package templates

import (
	"bytes"
	"errors"
	"io/fs"
	"testing"
)

// TestReadRequiredTemplates ensures required templates are embedded and ASCII.
func TestReadRequiredTemplates(t *testing.T) {
	for _, name := range Required() {
		data, err := Read(name)
		if err != nil {
			t.Fatalf("expected template %s to load: %v", name, err)
		}
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) == 0 && name != "roles/default.md" {
			t.Fatalf("expected template %s to be non-empty", name)
		}
		if !isASCII(data) {
			t.Fatalf("expected template %s to be ASCII", name)
		}
	}
}

// TestReadMissingTemplate returns a not-found error for unknown templates.
func TestReadMissingTemplate(t *testing.T) {
	_, err := Read("bootstrap/missing.md")
	if err == nil {
		t.Fatal("expected error for missing template")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("expected not-exist error, got %v", err)
	}
}

// TestReadInvalidName rejects invalid lookup keys.
func TestReadInvalidName(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"/bootstrap/asr.md",
		"bootstrap//asr.md",
		"bootstrap/../planning/task.md",
		"bootstrap/./asr.md",
		"planning\\task.md",
		"other/task.md",
	}
	for _, name := range cases {
		if _, err := Read(name); err == nil {
			t.Fatalf("expected error for invalid name %q", name)
		}
	}
}

// isASCII reports whether all bytes are valid ASCII characters.
func isASCII(data []byte) bool {
	for _, b := range data {
		if b > 0x7f {
			return false
		}
	}
	return true
}
