// Package run provides branch lifecycle management for task execution.
package run

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/index"
)

// BranchLifecycleManager handles creation, management, and cleanup of task branches.
type BranchLifecycleManager struct {
	repoRoot string
	auditor  *audit.Logger
}

// NewBranchLifecycleManager creates a new branch lifecycle manager.
func NewBranchLifecycleManager(repoRoot string, auditor *audit.Logger) *BranchLifecycleManager {
	return &BranchLifecycleManager{
		repoRoot: repoRoot,
		auditor:  auditor,
	}
}

// CreateTaskBranch creates a new branch for a task when it transitions to open state.
func (blm *BranchLifecycleManager) CreateTaskBranch(task index.Task, baseBranch string) error {
	if strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("task ID is required")
	}
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main" // Default to main branch
	}

	branchName := TaskBranchName(task)

	// Check if branch exists
	exists, err := blm.BranchExists(branchName)
	if err != nil {
		return fmt.Errorf("check if branch exists: %w", err)
	}
	if exists {
		// Branch already exists, nothing to do
		return nil
	}

	// Ensure we're on the base branch.
	if err := blm.runGit("checkout", baseBranch); err != nil {
		return fmt.Errorf("checkout base branch %s: %w", baseBranch, err)
	}

	// Create the new branch
	if err := blm.runGit("checkout", "-b", branchName); err != nil {
		return fmt.Errorf("create branch %s: %w", branchName, err)
	}

	// Log branch creation
	if blm.auditor != nil {
		_ = blm.auditor.LogBranchCreate(task.ID, string(task.Role), branchName, baseBranch)
	}

	return nil
}

// PrepareBaseBranch checks out the base branch.
// This should be called before batch branch creation operations to ensure
// all branches are created from the same consistent base.
func (blm *BranchLifecycleManager) PrepareBaseBranch(baseBranch string) error {
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}

	// Checkout the base branch
	if err := blm.runGit("checkout", baseBranch); err != nil {
		return fmt.Errorf("checkout base branch %s: %w", baseBranch, err)
	}

	return nil
}

// CreateTaskBranchWithoutCheckout creates a new branch for a task without checking it out.
// The base branch must already be checked out and up-to-date before calling this method.
// This is useful for batch branch creation where you want to create multiple branches
// without repeatedly switching between them.
//
// Use PrepareBaseBranch() before calling this in a loop.
func (blm *BranchLifecycleManager) CreateTaskBranchWithoutCheckout(task index.Task, baseBranch string) error {
	if strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("task ID is required")
	}
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main"
	}

	branchName := TaskBranchName(task)

	// Check if branch already exists
	exists, err := blm.BranchExists(branchName)
	if err != nil {
		return fmt.Errorf("check if branch exists: %w", err)
	}
	if exists {
		return nil
	}

	// Create branch without checkout: git branch <name> <base>
	if err := blm.runGit("branch", branchName, baseBranch); err != nil {
		return fmt.Errorf("create branch %s from %s: %w", branchName, baseBranch, err)
	}

	// Log branch creation
	if blm.auditor != nil {
		_ = blm.auditor.LogBranchCreate(task.ID, string(task.Role), branchName, baseBranch)
	}

	return nil
}

// CheckoutBranch checks out the specified branch.
func (blm *BranchLifecycleManager) CheckoutBranch(branch string) error {
	if strings.TrimSpace(branch) == "" {
		return fmt.Errorf("branch name is required")
	}
	return blm.runGit("checkout", branch)
}

// CleanupTaskBranch removes a task branch after successful completion.
// This should be called after the task has been successfully merged to main.
func (blm *BranchLifecycleManager) CleanupTaskBranch(task index.Task) error {
	if strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("task ID is required")
	}

	branchName := TaskBranchName(task)

	// Check if branch exists
	exists, err := blm.BranchExists(branchName)
	if err != nil {
		return fmt.Errorf("check if branch exists: %w", err)
	}
	if !exists {
		// Branch doesn't exist, nothing to clean up
		return nil
	}

	// Switch to main branch before deleting the task branch
	if err := blm.runGit("checkout", "main"); err != nil {
		return fmt.Errorf("checkout main branch: %w", err)
	}

	// Delete the task branch
	if err := blm.runGit("branch", "-d", branchName); err != nil {
		// If normal delete fails, try force delete
		if forceErr := blm.runGit("branch", "-D", branchName); forceErr != nil {
			return fmt.Errorf("delete branch %s: %w (force delete also failed: %v)", branchName, err, forceErr)
		}
	}

	// Log branch deletion
	if blm.auditor != nil {
		_ = blm.auditor.LogBranchDelete(task.ID, string(task.Role), branchName)
	}

	return nil
}

// EnsureTaskBranch ensures a task branch exists and is properly set up.
// This is used during resume operations or when a task needs to be worked on.
// NOTE: This method WILL checkout the task branch, leaving you on that branch.
// For batch branch creation without checkout, use CreateTaskBranchWithoutCheckout.
func (blm *BranchLifecycleManager) EnsureTaskBranch(task index.Task, baseBranch string) error {
	if strings.TrimSpace(task.ID) == "" {
		return fmt.Errorf("task ID is required")
	}
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = "main" // Default to main branch
	}

	branchName := TaskBranchName(task)

	// Check if branch exists
	exists, err := blm.BranchExists(branchName)
	if err != nil {
		return fmt.Errorf("check if branch exists: %w", err)
	}

	if !exists {
		// Branch doesn't exist, create it
		return blm.CreateTaskBranch(task, baseBranch)
	}

	// Branch exists, ensure we're on it
	if err := blm.runGit("checkout", branchName); err != nil {
		return fmt.Errorf("checkout task branch %s: %w", branchName, err)
	}

	return nil
}

// GetTaskBranchName returns the deterministic branch name for a task.
func (blm *BranchLifecycleManager) GetTaskBranchName(task index.Task) string {
	return TaskBranchName(task)
}

// BranchExists checks if a branch exists in the repository.
func (blm *BranchLifecycleManager) BranchExists(branchName string) (bool, error) {
	if strings.TrimSpace(branchName) == "" {
		return false, fmt.Errorf("branch name is required")
	}

	err := blm.runGit("show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
	if err == nil {
		return true, nil
	}

	// Check if this is a "not found" error (exit code 1)
	if strings.Contains(err.Error(), "exit status 1") {
		return false, nil
	}

	return false, fmt.Errorf("check branch existence: %w", err)
}

// runGit executes a git command in the repository root.
func (blm *BranchLifecycleManager) runGit(args ...string) error {
	if strings.TrimSpace(blm.repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if len(args) == 0 {
		return fmt.Errorf("git arguments are required")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = blm.repoRoot

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
