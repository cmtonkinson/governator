// Package worktree tests worktree management behavior.
package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestWorktreePathDeterministic verifies deterministic paths per task attempt.
func TestWorktreePathDeterministic(t *testing.T) {
	repoRoot := t.TempDir()
	manager, err := NewManager(repoRoot)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	first, err := manager.WorktreePath("T-014", 1)
	if err != nil {
		t.Fatalf("WorktreePath attempt 1 error: %v", err)
	}
	wantFirst := filepath.Join(repoRoot, "_governator", "_local-state", "worktrees", "T-014")
	if first != wantFirst {
		t.Fatalf("WorktreePath attempt 1 = %q, want %q", first, wantFirst)
	}

	second, err := manager.WorktreePath("T-014", 2)
	if err != nil {
		t.Fatalf("WorktreePath attempt 2 error: %v", err)
	}
	wantSecond := filepath.Join(repoRoot, "_governator", "_local-state", "worktrees", "T-014-attempt-2")
	if second != wantSecond {
		t.Fatalf("WorktreePath attempt 2 = %q, want %q", second, wantSecond)
	}
}

// TestEnsureWorktreeCreatesWorktree verifies worktrees are created for new tasks.
func TestEnsureWorktreeCreatesWorktree(t *testing.T) {
	repoRoot := initRepo(t)
	branch := "gov/T-001"
	runGit(t, repoRoot, "branch", branch)

	manager, err := NewManager(repoRoot)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	result, err := manager.EnsureWorktree(Spec{
		TaskID:  "T-001",
		Attempt: 1,
		Branch:  branch,
	})
	if err != nil {
		t.Fatalf("EnsureWorktree error: %v", err)
	}
	if result.Reused {
		t.Fatal("expected created worktree, got reused")
	}

	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("expected worktree at %s: %v", result.Path, err)
	}
	current := strings.TrimSpace(runGit(t, result.Path, "rev-parse", "--abbrev-ref", "HEAD"))
	if current != branch {
		t.Fatalf("worktree branch = %q, want %q", current, branch)
	}
	if result.RelativePath != "_governator/_local-state/worktrees/T-001" {
		t.Fatalf("relative path = %q, want %q", result.RelativePath, "_governator/_local-state/worktrees/T-001")
	}
}

// TestEnsureWorktreeReusePreservesChanges verifies reuse preserves uncommitted changes.
func TestEnsureWorktreeReusePreservesChanges(t *testing.T) {
	repoRoot := initRepo(t)
	branch := "gov/T-002"
	runGit(t, repoRoot, "branch", branch)

	manager, err := NewManager(repoRoot)
	if err != nil {
		t.Fatalf("NewManager error: %v", err)
	}

	first, err := manager.EnsureWorktree(Spec{
		TaskID:  "T-002",
		Attempt: 1,
		Branch:  branch,
	})
	if err != nil {
		t.Fatalf("EnsureWorktree first error: %v", err)
	}

	notePath := filepath.Join(first.Path, "note.txt")
	if err := os.WriteFile(notePath, []byte("in-progress"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	second, err := manager.EnsureWorktree(Spec{
		TaskID:  "T-002",
		Attempt: 1,
		Branch:  branch,
	})
	if err != nil {
		t.Fatalf("EnsureWorktree second error: %v", err)
	}
	if !second.Reused {
		t.Fatal("expected reused worktree")
	}
	if _, err := os.Stat(notePath); err != nil {
		t.Fatalf("expected preserved note %s: %v", notePath, err)
	}
}

// initRepo initializes a git repository with a single commit.
func initRepo(t *testing.T) string {
	t.Helper()

	repoRoot := t.TempDir()
	runGit(t, repoRoot, "init")
	runGit(t, repoRoot, "config", "user.email", "test@example.com")
	runGit(t, repoRoot, "config", "user.name", "Governator Test")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("test"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGit(t, repoRoot, "add", "README.md")
	runGit(t, repoRoot, "commit", "-m", "init")
	runGit(t, repoRoot, "branch", "-M", "main")

	return repoRoot
}

// runGit executes a git command in the provided directory.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()

	cmd := execCommand("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, string(output))
	}
	return string(output)
}

// execCommand wraps exec.Command for testability.
func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
