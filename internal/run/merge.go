// Package run provides merge flow operations for transitioning reviewed tasks to done.
package run

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/index"
)

// MergeFlowInput defines the inputs required for the review merge flow.
type MergeFlowInput struct {
	RepoRoot     string
	WorktreePath string
	Task         index.Task
	MainBranch   string
	Auditor      *audit.Logger
}

// MergeFlowResult captures the outcome of the review merge flow.
type MergeFlowResult struct {
	Success       bool
	NewState      index.TaskState
	ConflictError string
}

// ExecuteReviewMergeFlow performs the git rebase and merge operations for a reviewed task.
// This implements the flow: rebase on main → squash commits → fast-forward merge → done
// On conflict: mark as conflict state for manual resolution.
func ExecuteReviewMergeFlow(input MergeFlowInput) (MergeFlowResult, error) {
	if strings.TrimSpace(input.RepoRoot) == "" {
		return MergeFlowResult{}, fmt.Errorf("repo root is required")
	}
	if strings.TrimSpace(input.WorktreePath) == "" {
		return MergeFlowResult{}, fmt.Errorf("worktree path is required")
	}
	if strings.TrimSpace(input.Task.ID) == "" {
		return MergeFlowResult{}, fmt.Errorf("task ID is required")
	}
	if strings.TrimSpace(input.MainBranch) == "" {
		input.MainBranch = "main" // Default to main branch
	}

	// Step 1: Fetch latest main to ensure we have up-to-date refs
	if err := runGitInWorktree(input.WorktreePath, "fetch", "origin", input.MainBranch); err != nil {
		return MergeFlowResult{}, fmt.Errorf("fetch main branch: %w", err)
	}

	// Step 2: Attempt rebase on main
	rebaseErr := runGitInWorktree(input.WorktreePath, "rebase", "origin/"+input.MainBranch)
	if rebaseErr != nil {
		// Check if this is a rebase conflict
		if isRebaseConflict(rebaseErr) {
			// Abort the rebase to leave worktree in clean state
			_ = runGitInWorktree(input.WorktreePath, "rebase", "--abort")
			
			// Log the conflict to audit
			if input.Auditor != nil {
				_ = input.Auditor.LogTaskTransition(
					input.Task.ID,
					string(input.Task.Role),
					string(index.TaskStateTested),
					string(index.TaskStateConflict),
				)
			}

			return MergeFlowResult{
				Success:       false,
				NewState:      index.TaskStateConflict,
				ConflictError: fmt.Sprintf("rebase conflict with %s: %v", input.MainBranch, rebaseErr),
			}, nil
		}
		// Non-conflict rebase error
		return MergeFlowResult{}, fmt.Errorf("rebase failed: %w", rebaseErr)
	}

	// Step 3: Switch to main branch in repo root for merge
	if err := runGitInRepo(input.RepoRoot, "checkout", input.MainBranch); err != nil {
		return MergeFlowResult{}, fmt.Errorf("checkout main branch: %w", err)
	}

	// Step 4: Perform squash merge of task branch
	taskBranch := fmt.Sprintf("task-%s", input.Task.ID)
	mergeErr := runGitInRepo(input.RepoRoot, "merge", "--squash", taskBranch)
	if mergeErr != nil {
		// Check if this is a merge conflict
		if isMergeConflict(mergeErr) {
			// Reset to clean state
			_ = runGitInRepo(input.RepoRoot, "reset", "--hard", "HEAD")
			
			// Log the conflict to audit
			if input.Auditor != nil {
				_ = input.Auditor.LogTaskTransition(
					input.Task.ID,
					string(input.Task.Role),
					string(index.TaskStateTested),
					string(index.TaskStateConflict),
				)
			}

			return MergeFlowResult{
				Success:       false,
				NewState:      index.TaskStateConflict,
				ConflictError: fmt.Sprintf("merge conflict with %s: %v", input.MainBranch, mergeErr),
			}, nil
		}
		// Non-conflict merge error
		return MergeFlowResult{}, fmt.Errorf("squash merge failed: %w", mergeErr)
	}

	// Step 5: Commit the squashed changes
	commitMsg := fmt.Sprintf("governator: %s - %s", input.Task.ID, input.Task.Title)
	if err := runGitInRepo(input.RepoRoot, "commit", "-m", commitMsg); err != nil {
		return MergeFlowResult{}, fmt.Errorf("commit squashed changes: %w", err)
	}

	// Step 6: Clean up task branch after successful merge
	branchManager := NewBranchLifecycleManager(input.RepoRoot, input.Auditor)
	if err := branchManager.CleanupTaskBranch(input.Task); err != nil {
		// Log warning but don't fail the merge - branch cleanup is not critical
		if input.Auditor != nil {
			_ = input.Auditor.Log(audit.Entry{
				TaskID: input.Task.ID,
				Role:   string(input.Task.Role),
				Event:  "branch.cleanup.warning",
				Fields: []audit.Field{
					{Key: "error", Value: err.Error()},
				},
			})
		}
	}

	// Step 7: Log successful transition to audit
	if input.Auditor != nil {
		_ = input.Auditor.LogTaskTransition(
			input.Task.ID,
			string(input.Task.Role),
			string(index.TaskStateTested),
			string(index.TaskStateDone),
		)
	}

	return MergeFlowResult{
		Success:  true,
		NewState: index.TaskStateDone,
	}, nil
}

// ExecuteConflictResolutionMergeFlow handles merge flow for resolved tasks.
// This is similar to the review merge flow but starts from resolved state.
func ExecuteConflictResolutionMergeFlow(input MergeFlowInput) (MergeFlowResult, error) {
	if input.Task.State != index.TaskStateResolved {
		return MergeFlowResult{}, fmt.Errorf("task must be in resolved state, got %s", input.Task.State)
	}

	// Use the same merge flow logic but with different state transitions
	result, err := ExecuteReviewMergeFlow(input)
	if err != nil {
		return result, err
	}

	// Clean up branch if merge was successful
	if result.Success {
		branchManager := NewBranchLifecycleManager(input.RepoRoot, input.Auditor)
		if err := branchManager.CleanupTaskBranch(input.Task); err != nil {
			// Log warning but don't fail the merge - branch cleanup is not critical
			if input.Auditor != nil {
				_ = input.Auditor.Log(audit.Entry{
					TaskID: input.Task.ID,
					Role:   string(input.Task.Role),
					Event:  "branch.cleanup.warning",
					Fields: []audit.Field{
						{Key: "error", Value: err.Error()},
					},
				})
			}
		}
	}

	// Override audit logging for resolved → done/conflict transitions
	if input.Auditor != nil {
		if result.Success {
			_ = input.Auditor.LogTaskTransition(
				input.Task.ID,
				string(input.Task.Role),
				string(index.TaskStateResolved),
				string(index.TaskStateDone),
			)
		} else if result.NewState == index.TaskStateConflict {
			_ = input.Auditor.LogTaskTransition(
				input.Task.ID,
				string(input.Task.Role),
				string(index.TaskStateResolved),
				string(index.TaskStateConflict),
			)
		}
	}

	return result, nil
}

// runGitInWorktree executes a git command in the specified worktree directory.
func runGitInWorktree(worktreePath string, args ...string) error {
	if strings.TrimSpace(worktreePath) == "" {
		return fmt.Errorf("worktree path is required")
	}
	if len(args) == 0 {
		return fmt.Errorf("git arguments are required")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = worktreePath
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// runGitInRepo executes a git command in the repository root.
func runGitInRepo(repoRoot string, args ...string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if len(args) == 0 {
		return fmt.Errorf("git arguments are required")
	}

	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// isRebaseConflict determines if a git error indicates a rebase conflict.
func isRebaseConflict(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "conflict") || 
		   strings.Contains(errStr, "could not apply") ||
		   strings.Contains(errStr, "merge conflict")
}

// isMergeConflict determines if a git error indicates a merge conflict.
func isMergeConflict(err error) bool {
	if err == nil {
		return false
	}
	errStr := strings.ToLower(err.Error())
	return strings.Contains(errStr, "conflict") || 
		   strings.Contains(errStr, "automatic merge failed") ||
		   strings.Contains(errStr, "merge conflict")
}