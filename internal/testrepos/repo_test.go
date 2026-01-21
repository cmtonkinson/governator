package testrepos

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewCreatesGitRepo(t *testing.T) {
	t.Parallel()

	repo := New(t)

	if _, err := os.Stat(filepath.Join(repo.Root, ".git")); err != nil {
		t.Fatalf("expected .git directory: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repo.Root, "README.md")); err != nil {
		t.Fatalf("expected README file: %v", err)
	}

	if got := strings.TrimSpace(repo.RunGit(t, "log", "--oneline")); got == "" {
		t.Fatalf("expected git log to contain initial commit, got empty output")
	}
}

func TestCleanupHandlesMissingDirectory(t *testing.T) {
	t.Parallel()

	missing := filepath.Join(t.TempDir(), "missing")
	if err := os.RemoveAll(missing); err != nil && !os.IsNotExist(err) {
		t.Fatalf("prepare missing directory: %v", err)
	}

	repo := &TempRepo{Root: missing}
	if err := repo.Cleanup(); err != nil {
		t.Fatalf("cleanup with missing directory should succeed: %v", err)
	}
}

func TestCleanupDeletesRepo(t *testing.T) {
	t.Parallel()

	parent := t.TempDir()
	repo := &TempRepo{Root: filepath.Join(parent, "repo")}
	if err := os.MkdirAll(repo.Root, 0o755); err != nil {
		t.Fatalf("create repo dir: %v", err)
	}

	if err := repo.Cleanup(); err != nil {
		t.Fatalf("cleanup repo: %v", err)
	}

	if _, err := os.Stat(repo.Root); err == nil || !os.IsNotExist(err) {
		t.Fatalf("repo still exists after cleanup")
	}
}
