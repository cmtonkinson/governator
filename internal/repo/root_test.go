// Tests for git repository root discovery.
package repo

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiscoverRootFromNestedDir verifies nested paths resolve the repo root.
func TestDiscoverRootFromNestedDir(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, gitDirName)
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", gitDir, err)
	}

	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", nested, err)
	}

	got, err := DiscoverRoot(nested)
	if err != nil {
		t.Fatalf("discover root: %v", err)
	}
	want := canonicalPath(t, root)
	if got != want {
		t.Fatalf("repo root = %s, want %s", got, want)
	}
}

// TestDiscoverRootWithGitFile verifies a .git file is treated as a repo root marker.
func TestDiscoverRootWithGitFile(t *testing.T) {
	root := t.TempDir()
	gitFile := filepath.Join(root, gitDirName)
	if err := os.WriteFile(gitFile, []byte("gitdir: /tmp/nowhere\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", gitFile, err)
	}

	got, err := DiscoverRoot(root)
	if err != nil {
		t.Fatalf("discover root: %v", err)
	}
	want := canonicalPath(t, root)
	if got != want {
		t.Fatalf("repo root = %s, want %s", got, want)
	}
}

// TestDiscoverRootFromCWD verifies discovery uses the current working directory.
func TestDiscoverRootFromCWD(t *testing.T) {
	root := t.TempDir()
	gitDir := filepath.Join(root, gitDirName)
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", gitDir, err)
	}

	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", nested, err)
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(nested); err != nil {
		t.Fatalf("chdir %s: %v", nested, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	got, err := DiscoverRootFromCWD()
	if err != nil {
		t.Fatalf("discover root: %v", err)
	}
	want := canonicalPath(t, root)
	if got != want {
		t.Fatalf("repo root = %s, want %s", got, want)
	}
}

// TestDiscoverRootMissingRepo verifies a clear error is returned outside a repo.
func TestDiscoverRootMissingRepo(t *testing.T) {
	dir := t.TempDir()

	_, err := DiscoverRoot(dir)
	if err == nil {
		t.Fatal("expected error")
	}
	if !errorsIs(err, ErrRepoNotFound) {
		t.Fatalf("expected ErrRepoNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "git init") {
		t.Fatalf("expected guidance to run git init, got %q", err.Error())
	}
}

// errorsIs wraps errors.Is for test assertions.
func errorsIs(err error, target error) bool {
	return errors.Is(err, target)
}

// canonicalPath resolves symlinks to provide a stable comparison path.
func canonicalPath(t *testing.T, path string) string {
	t.Helper()

	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatalf("eval symlinks %s: %v", path, err)
	}
	return resolved
}
