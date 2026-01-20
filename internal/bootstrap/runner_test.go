// Package bootstrap provides tests for Power Six bootstrap artifact creation.
package bootstrap

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/templates"
)

// TestRunCreatesArtifacts ensures bootstrap writes all required and optional artifacts.
func TestRunCreatesArtifacts(t *testing.T) {
	t.Helper()
	root := t.TempDir()

	result, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	wantCount := len(Artifacts())
	if len(result.Written) != wantCount {
		t.Fatalf("Written count = %d, want %d", len(result.Written), wantCount)
	}

	for _, artifact := range Artifacts() {
		path := filepath.Join(root, docsDirName, artifact.Name)
		data := readFile(t, path)
		templateData := readTemplate(t, artifact.Template)
		if !bytes.Equal(data, templateData) {
			t.Fatalf("artifact %s contents mismatch template", artifact.Name)
		}
	}
}

// TestRunSkipsExistingArtifacts ensures existing artifacts are not overwritten.
func TestRunSkipsExistingArtifacts(t *testing.T) {
	t.Helper()
	root := t.TempDir()

	existing := Artifacts()[0]
	existingPath := filepath.Join(root, docsDirName, existing.Name)
	writeFile(t, existingPath, "custom content")

	result, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	got := string(readFile(t, existingPath))
	if got != "custom content" {
		t.Fatalf("existing content = %q, want %q", got, "custom content")
	}

	if len(result.Written) != len(Artifacts())-1 {
		t.Fatalf("Written count = %d, want %d", len(result.Written), len(Artifacts())-1)
	}
}

// TestRunIdempotent ensures a second run does not overwrite edited artifacts.
func TestRunIdempotent(t *testing.T) {
	t.Helper()
	root := t.TempDir()

	if _, err := Run(root, Options{}); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	edited := Artifacts()[1]
	editedPath := filepath.Join(root, docsDirName, edited.Name)
	writeFile(t, editedPath, "operator edits")

	result, err := Run(root, Options{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	got := string(readFile(t, editedPath))
	if got != "operator edits" {
		t.Fatalf("edited content = %q, want %q", got, "operator edits")
	}

	if len(result.Written) != 0 {
		t.Fatalf("Written count = %d, want 0", len(result.Written))
	}
	if len(result.Skipped) != len(Artifacts()) {
		t.Fatalf("Skipped count = %d, want %d", len(result.Skipped), len(Artifacts()))
	}
}

// TestRunUsesLocalTemplateOverride ensures repo-local templates are preferred.
func TestRunUsesLocalTemplateOverride(t *testing.T) {
	t.Helper()
	root := t.TempDir()

	override := "override template"
	overridePath := filepath.Join(root, templatesDirName, "bootstrap", "asr.md")
	writeFile(t, overridePath, override)

	if _, err := Run(root, Options{}); err != nil {
		t.Fatalf("Run error: %v", err)
	}

	path := filepath.Join(root, docsDirName, "asr.md")
	got := string(readFile(t, path))
	if got != override {
		t.Fatalf("override content = %q, want %q", got, override)
	}
}

// readTemplate loads the embedded template contents for a lookup key.
func readTemplate(t *testing.T, name string) []byte {
	t.Helper()
	data, err := templates.Read(name)
	if err != nil {
		t.Fatalf("templates.Read error: %v", err)
	}
	return data
}

// readFile reads a file from disk and fails the test on error.
func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	return data
}

// writeFile writes a file after ensuring parent directories exist.
func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	writeDir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
}

// writeDir ensures the directory exists.
func writeDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
}
