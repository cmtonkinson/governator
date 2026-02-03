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

// TestFileValidationEdgeCases tests symlinks, permissions, and special file types
func TestFileValidationEdgeCases(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("symlink_to_file_valid", func(t *testing.T) {
		target := filepath.Join(repoRoot, "docs", "target.md")
		writeTestFile(t, target, "target content")

		symlink := filepath.Join(repoRoot, "docs", "link.md")
		if err := os.Symlink(target, symlink); err != nil {
			t.Fatalf("create symlink: %v", err)
		}

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/link.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass for symlink, got message: %q", message)
		}
	})

	t.Run("broken_symlink_fails", func(t *testing.T) {
		symlink := filepath.Join(repoRoot, "docs", "broken.md")
		nonexistent := filepath.Join(repoRoot, "docs", "nonexistent.md")
		if err := os.MkdirAll(filepath.Dir(symlink), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.Symlink(nonexistent, symlink); err != nil {
			t.Fatalf("create broken symlink: %v", err)
		}

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/broken.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "does not exist") {
			t.Fatalf("expected broken symlink failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("file_no_read_permission_fails", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("skipping permission test when running as root")
		}

		unreadable := filepath.Join(repoRoot, "docs", "unreadable.md")
		writeTestFile(t, unreadable, "content")

		if err := os.Chmod(unreadable, 0000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		defer os.Chmod(unreadable, 0644) // Restore for cleanup

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/unreadable.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "not readable") {
			t.Fatalf("expected unreadable file failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("large_file_with_regex_valid", func(t *testing.T) {
		// Create a ~10MB file
		large := filepath.Join(repoRoot, "docs", "large.txt")
		content := strings.Repeat("This is a test line with PATTERN marker\n", 250000) // ~10MB
		writeTestFile(t, large, content)

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type:      "file",
			Path:      "docs/large.txt",
			FileRegex: "PATTERN",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected large file validation to pass, got message: %q", message)
		}
	})

	t.Run("unicode_content_with_regex_valid", func(t *testing.T) {
		unicode := filepath.Join(repoRoot, "docs", "unicode.md")
		content := "# 日本語タイトル\n\nHello 世界 🌍\n\nPattern: MATCH"
		writeTestFile(t, unicode, content)

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type:      "file",
			Path:      "docs/unicode.md",
			FileRegex: "MATCH",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected unicode file validation to pass, got message: %q", message)
		}
	})

	t.Run("file_with_null_bytes_regex_match", func(t *testing.T) {
		binary := filepath.Join(repoRoot, "docs", "binary.dat")
		content := "text\x00PATTERN\x00more"
		writeTestFile(t, binary, content)

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type:      "file",
			Path:      "docs/binary.dat",
			FileRegex: "PATTERN",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected binary file with pattern to pass, got message: %q", message)
		}
	})
}

// TestDirectoryValidationEdgeCases tests symlinks and permissions for directories
func TestDirectoryValidationEdgeCases(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("symlink_to_directory_valid", func(t *testing.T) {
		target := filepath.Join(repoRoot, "docs", "target-dir")
		if err := os.MkdirAll(target, 0o755); err != nil {
			t.Fatalf("mkdir target: %v", err)
		}

		symlink := filepath.Join(repoRoot, "docs", "link-dir")
		if err := os.Symlink(target, symlink); err != nil {
			t.Fatalf("create symlink: %v", err)
		}

		ok, message, err := engine.runDirectoryValidation(PlanningValidationSpec{
			Type: "directory",
			Path: "docs/link-dir",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected validation to pass for directory symlink, got message: %q", message)
		}
	})

	t.Run("directory_no_read_permission_fails", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("skipping permission test when running as root")
		}

		unreadable := filepath.Join(repoRoot, "docs", "unreadable-dir")
		if err := os.MkdirAll(unreadable, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		if err := os.Chmod(unreadable, 0000); err != nil {
			t.Fatalf("chmod: %v", err)
		}
		defer os.Chmod(unreadable, 0755) // Restore for cleanup

		ok, message, err := engine.runDirectoryValidation(PlanningValidationSpec{
			Type: "directory",
			Path: "docs/unreadable-dir",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "not readable") {
			t.Fatalf("expected unreadable directory failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("empty_directory_valid", func(t *testing.T) {
		empty := filepath.Join(repoRoot, "docs", "empty-dir")
		if err := os.MkdirAll(empty, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}

		ok, message, err := engine.runDirectoryValidation(PlanningValidationSpec{
			Type: "directory",
			Path: "docs/empty-dir",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected empty directory to be valid, got message: %q", message)
		}
	})
}

// TestGlobPatternEdgeCases tests glob patterns with special characters and edge cases
func TestGlobPatternEdgeCases(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("glob_no_matches_fails", func(t *testing.T) {
		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "nonexistent/*.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok || !strings.Contains(message, "no files match glob") {
			t.Fatalf("expected glob no match failure, got ok=%v message=%q", ok, message)
		}
	})

	t.Run("glob_with_brackets", func(t *testing.T) {
		writeTestFile(t, filepath.Join(repoRoot, "docs", "file1.md"), "content1")
		writeTestFile(t, filepath.Join(repoRoot, "docs", "file2.md"), "content2")
		writeTestFile(t, filepath.Join(repoRoot, "docs", "file3.md"), "content3")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "docs/file[12].md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected glob with brackets to pass, got message: %q", message)
		}
	})

	t.Run("glob_with_question_mark", func(t *testing.T) {
		writeTestFile(t, filepath.Join(repoRoot, "test", "fileA.txt"), "A")
		writeTestFile(t, filepath.Join(repoRoot, "test", "fileB.txt"), "B")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "test/file?.txt",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected glob with ? to pass, got message: %q", message)
		}
	})

	t.Run("glob_deeply_nested", func(t *testing.T) {
		deepPath := filepath.Join(repoRoot, "a", "b", "c", "d", "e", "deep.md")
		writeTestFile(t, deepPath, "deep content")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "a/b/c/d/e/*.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected deeply nested glob to pass, got message: %q", message)
		}
	})

	t.Run("glob_special_chars_in_filename", func(t *testing.T) {
		// Create files with special characters (that aren't glob meta-characters)
		writeTestFile(t, filepath.Join(repoRoot, "special", "file-with-dash.md"), "dash")
		writeTestFile(t, filepath.Join(repoRoot, "special", "file_with_underscore.md"), "underscore")
		writeTestFile(t, filepath.Join(repoRoot, "special", "file.with.dots.md"), "dots")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "special/*.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected glob with special chars to pass, got message: %q", message)
		}
	})

	t.Run("glob_regex_validation_all_must_match", func(t *testing.T) {
		writeTestFile(t, filepath.Join(repoRoot, "regex", "good1.md"), "MATCH")
		writeTestFile(t, filepath.Join(repoRoot, "regex", "good2.md"), "MATCH")
		writeTestFile(t, filepath.Join(repoRoot, "regex", "bad.md"), "no match")

		// Should fail because bad.md doesn't match the regex
		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type:      "file",
			Path:      "regex/*.md",
			FileRegex: "MATCH",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected glob regex validation to fail when one file doesn't match")
		}
		if !strings.Contains(message, "does not match regex") {
			t.Fatalf("expected regex failure message, got: %q", message)
		}
	})
}

// TestPathResolutionEdgeCases tests various path formats and resolution
func TestPathResolutionEdgeCases(t *testing.T) {
	repoRoot := t.TempDir()
	engine := NewValidationEngine(repoRoot)

	t.Run("path_with_spaces", func(t *testing.T) {
		spacePath := filepath.Join(repoRoot, "path with spaces", "file with spaces.md")
		writeTestFile(t, spacePath, "content")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "path with spaces/file with spaces.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected path with spaces to pass, got message: %q", message)
		}
	})

	t.Run("path_with_unicode", func(t *testing.T) {
		unicodePath := filepath.Join(repoRoot, "文档", "文件.md")
		writeTestFile(t, unicodePath, "content")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "文档/文件.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected unicode path to pass, got message: %q", message)
		}
	})

	t.Run("path_relative_current_dir", func(t *testing.T) {
		currentFile := filepath.Join(repoRoot, "file.md")
		writeTestFile(t, currentFile, "content")

		ok, message, err := engine.runFileValidation(PlanningValidationSpec{
			Type: "file",
			Path: "file.md",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected current dir file to pass, got message: %q", message)
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
