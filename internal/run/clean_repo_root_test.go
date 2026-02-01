// File: clean_repo_root_test.go
// Purpose: Verify ensureCleanRepoRoot ignores planning index metadata while still
// rejecting unrelated dirty state.
package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCleanRepoRootIgnoresPlanningIndex(t *testing.T) {
	repoDir := t.TempDir()
	if err := runGitInRepo(repoDir, "init"); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	taskIndexPath := filepath.Join(repoDir, "_governator", "task-index.json")
	if err := os.MkdirAll(filepath.Dir(taskIndexPath), 0o755); err != nil {
		t.Fatalf("create task index dir: %v", err)
	}
	if err := os.WriteFile(taskIndexPath, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("write task index: %v", err)
	}
	if err := runGitInRepo(repoDir, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runGitInRepo(repoDir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if err := os.WriteFile(taskIndexPath, []byte(`{"ok":false}`), 0o644); err != nil {
		t.Fatalf("modify task index: %v", err)
	}
	if err := ensureCleanRepoRoot(repoDir); err != nil {
		t.Fatalf("expected clean repo root despite task index change, got: %v", err)
	}
}

func TestEnsureCleanRepoRootRejectsOtherChanges(t *testing.T) {
	repoDir := t.TempDir()
	if err := runGitInRepo(repoDir, "init"); err != nil {
		t.Fatalf("init repo: %v", err)
	}
	readmePath := filepath.Join(repoDir, "README.md")
	if err := os.WriteFile(readmePath, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	if err := runGitInRepo(repoDir, "add", "-A"); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runGitInRepo(repoDir, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "init"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	if err := os.WriteFile(readmePath, []byte("dirty"), 0o644); err != nil {
		t.Fatalf("modify README: %v", err)
	}
	if err := ensureCleanRepoRoot(repoDir); err == nil {
		t.Fatal("expected ensureCleanRepoRoot to reject non-ignored changes")
	}
}
