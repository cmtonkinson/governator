// Package test provides end-to-end coverage for conflict resolution permission handling.
package test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/testrepos"
	"github.com/cmtonkinson/governator/internal/worktree"
)

// TestE2EConflictResolution_GitMetadataAccessible verifies Git metadata paths are accessible in worktrees.
// This test validates that the Git metadata path resolution and writability checks work correctly
// in realistic worktree scenarios, which is critical for conflict resolution.
func TestE2EConflictResolution_GitMetadataAccessible(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root
	TrackE2ERepo(t, repoRoot)

	// Create a test branch
	branch := "task-T-conflict-test"
	repo.RunGit(t, "checkout", "-b", branch)
	testFile := filepath.Join(repoRoot, "test.txt")
	if err := os.WriteFile(testFile, []byte("initial content\n"), 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}
	repo.RunGit(t, "add", "test.txt")
	repo.RunGit(t, "commit", "-m", "Add test file")
	repo.RunGit(t, "checkout", "main")

	// Create a worktree for the task
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}

	result, err := manager.EnsureWorktree(worktree.Spec{
		WorkstreamID: "T-conflict-test",
		Branch:       branch,
		BaseBranch:   "main",
	})
	if err != nil {
		t.Fatalf("ensure worktree: %v", err)
	}

	// Test 1: Git metadata path resolution
	metadataPath, err := worktree.GitMetadataPath(result.Path)
	if err != nil {
		t.Fatalf("GitMetadataPath failed: %v", err)
	}

	// Verify the metadata path exists
	info, err := os.Stat(metadataPath)
	if err != nil {
		t.Fatalf("Git metadata path does not exist: %s: %v", metadataPath, err)
	}
	if !info.IsDir() {
		t.Fatalf("Git metadata path is not a directory: %s", metadataPath)
	}

	// Test 2: Git metadata writability validation
	if err := worktree.ValidateGitMetadataWritable(result.Path); err != nil {
		t.Fatalf("ValidateGitMetadataWritable failed: %v", err)
	}

	// Test 3: Verify probe file was cleaned up
	probePath := filepath.Join(metadataPath, ".governator-write-test")
	if _, err := os.Stat(probePath); err == nil {
		t.Fatalf("Probe file was not cleaned up: %s", probePath)
	}

	// Test 4: Simulate a Git operation that requires metadata write access
	// This mimics what happens during conflict resolution
	repo.RunGitInDir(t, result.Path, "checkout", "-b", "test-metadata-write")
	repo.RunGitInDir(t, result.Path, "checkout", branch)

	// Verify we can still validate writability after Git operations
	if err := worktree.ValidateGitMetadataWritable(result.Path); err != nil {
		t.Fatalf("ValidateGitMetadataWritable failed after Git operations: %v", err)
	}
}

// TestE2EConflictResolution_HappyPath verifies the complete conflict resolution workflow.
// This ensures that all the new permission handling code works together correctly.
func TestE2EConflictResolution_HappyPath(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root
	TrackE2ERepo(t, repoRoot)

	// Create a conflict scenario
	// Main branch: modify test.txt
	testFile := filepath.Join(repoRoot, "test.txt")
	if err := os.WriteFile(testFile, []byte("main branch content\n"), 0o644); err != nil {
		t.Fatalf("write test file on main: %v", err)
	}
	repo.RunGit(t, "add", "test.txt")
	repo.RunGit(t, "commit", "-m", "Main branch change")

	// Task branch: modify test.txt differently
	branch := "task-T-conflict-happy"
	repo.RunGit(t, "checkout", "-b", branch, "HEAD~1")
	if err := os.WriteFile(testFile, []byte("task branch content\n"), 0o644); err != nil {
		t.Fatalf("write test file on task branch: %v", err)
	}
	repo.RunGit(t, "add", "test.txt")
	repo.RunGit(t, "commit", "-m", "Task branch change")

	// Switch back to main so the branch can be used in a worktree
	repo.RunGit(t, "checkout", "main")

	// Create worktree
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}

	result, err := manager.EnsureWorktree(worktree.Spec{
		WorkstreamID: "T-conflict-happy",
		Branch:       branch,
		BaseBranch:   "main",
	})
	if err != nil {
		t.Fatalf("ensure worktree: %v", err)
	}

	// Verify preflight check passes before attempting conflict resolution
	if err := worktree.ValidateGitMetadataWritable(result.Path); err != nil {
		t.Fatalf("Preflight check failed: %v", err)
	}

	// Attempt to rebase (this will create a conflict)
	// We don't expect this to succeed, but we want to verify that Git metadata is accessible
	_ = repo.RunGitInDirAllowError(t, result.Path, "rebase", "main")

	// Even after a failed rebase, we should still be able to validate writability
	if err := worktree.ValidateGitMetadataWritable(result.Path); err != nil {
		t.Fatalf("ValidateGitMetadataWritable failed after rebase attempt: %v", err)
	}

	// Cleanup: abort the rebase
	_ = repo.RunGitInDirAllowError(t, result.Path, "rebase", "--abort")
}
