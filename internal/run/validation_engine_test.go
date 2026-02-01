// Package run contains tests for planning validation behavior.
package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidationEngineFileValidation(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("file exists readable non-empty", func(t *testing.T) {
		path := filepath.Join(repoRoot, "docs", "ok.md")
		writeTestFile(t, path, "hello")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/ok.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass, got %q", message)
		}
	})

	t.Run("file empty fails", func(t *testing.T) {
		path := filepath.Join(repoRoot, "docs", "empty.md")
		writeTestFile(t, path, "")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/empty.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "file is empty") {
			t.Fatalf("expected empty file failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("directory fails", func(t *testing.T) {
		dir := filepath.Join(repoRoot, "docs", "dir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/dir",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "file validation requires a file") {
			t.Fatalf("expected directory failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("glob requires at least one match", func(t *testing.T) {
		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/missing*.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "no files match glob (file validation requires only files)") {
			t.Fatalf("expected glob missing failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("glob rejects directories even when files match", func(t *testing.T) {
		writeTestFile(t, filepath.Join(repoRoot, "docs", "adr1.md"), "ok")
		dir := filepath.Join(repoRoot, "docs", "adr-dir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/adr*",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "file validation requires only files") {
			t.Fatalf("expected glob directory failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("glob with only directories fails", func(t *testing.T) {
		dir := filepath.Join(repoRoot, "docs", "only-dir")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/only-*",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "file validation requires only files") {
			t.Fatalf("expected glob directory failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("glob applies regex to each match", func(t *testing.T) {
		writeTestFile(t, filepath.Join(repoRoot, "docs", "adr-good.md"), "MATCH")
		writeTestFile(t, filepath.Join(repoRoot, "docs", "adr-bad.md"), "nope")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type:      "file",
			Path:      "docs/adr-*.md",
			FileRegex: "MATCH",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "file content does not match regex") {
			t.Fatalf("expected regex failure, got ok=%v message=%q", ok, message)
		}
	})
}

func TestValidationEngineDirectoryValidation(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("directory exists readable", func(t *testing.T) {
		dir := filepath.Join(repoRoot, "docs", "ok")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		ok, message, err := engine.runDirectoryValidation(PlanningValidationSpec{
			Type: "directory",
			Path: "docs/ok",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected directory validation to pass, got %q", message)
		}
	})

	t.Run("directory glob requires at least one match", func(t *testing.T) {
		ok, message, err := engine.runDirectoryValidation(PlanningValidationSpec{
			Type: "directory",
			Path: "docs/missing*",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "no directories match glob (directory validation requires only directories)") {
			t.Fatalf("expected glob missing failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("directory glob rejects files", func(t *testing.T) {
		writeTestFile(t, filepath.Join(repoRoot, "docs", "dir-file"), "ok")
		dir := filepath.Join(repoRoot, "docs", "dir-ok")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		ok, message, err := engine.runDirectoryValidation(PlanningValidationSpec{
			Type: "directory",
			Path: "docs/dir*",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "directory validation requires only directories") {
			t.Fatalf("expected glob file failure, got ok=%v message=%q", ok, message)
		}
	})
}

func writeTestFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
