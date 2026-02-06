// Package run provides merge flow operations for transitioning reviewed tasks to done.
package run

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

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
// This implements the flow: rebase on main → squash in isolated worktree → update local main → done
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
	if strings.TrimSpace(input.Task.Title) == "" {
		return MergeFlowResult{}, fmt.Errorf("task title is required")
	}
	if err := ensureCleanWorktree(input.WorktreePath); err != nil {
		return MergeFlowResult{}, err
	}

	// Step 1: Fetch latest main to ensure we have up-to-date refs
	if err := fetchBranch(input.WorktreePath, "origin", input.MainBranch); err != nil {
		return MergeFlowResult{}, err
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

	// Step 3: Create an isolated merge worktree on the main branch.
	mergeWorktreePath, cleanupMergeWorktree, err := createMergeWorktree(input.RepoRoot, input.MainBranch, input.Task.ID)
	if err != nil {
		return MergeFlowResult{}, err
	}
	defer func() {
		if cleanupErr := cleanupMergeWorktree(); cleanupErr != nil && input.Auditor != nil {
			_ = input.Auditor.Log(audit.Entry{
				TaskID: input.Task.ID,
				Role:   string(input.Task.Role),
				Event:  "merge.worktree.cleanup.warning",
				Fields: []audit.Field{{Key: "error", Value: cleanupErr.Error()}},
			})
		}
	}()

	// Step 4: Perform squash merge of task branch in the isolated worktree.
	taskBranch := TaskBranchName(input.Task)
	mergeErr := runGitInWorktree(mergeWorktreePath, "merge", "--squash", taskBranch)
	if mergeErr != nil {
		// Check if this is a merge conflict
		if isMergeConflict(mergeErr) {
			// Reset the merge worktree to a clean state.
			_ = runGitInWorktree(mergeWorktreePath, "reset", "--hard", "HEAD")

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
	commitErr := runGitInWorktree(mergeWorktreePath, "commit", "-m", commitMsg)
	if commitErr != nil {
		lower := strings.ToLower(commitErr.Error())
		if !strings.Contains(lower, "nothing to commit") && !strings.Contains(lower, "working tree clean") {
			return MergeFlowResult{}, fmt.Errorf("commit squashed changes: %w", commitErr)
		}
	}

	// Step 5: Get the merge commit SHA from the merge worktree
	mergeCommit, err := getWorktreeCommit(mergeWorktreePath)
	if err != nil {
		return MergeFlowResult{}, fmt.Errorf("get merge commit: %w", err)
	}

	// Update main worktree to the merge commit
	if err := runGitInRepo(input.RepoRoot, "reset", "--hard", mergeCommit); err != nil {
		return MergeFlowResult{}, fmt.Errorf("update main to merge commit: %w", err)
	}

	// Step 6: Remove task worktree to allow branch deletion
	worktreePath := filepath.Join(input.RepoRoot, "_governator", "_local-state", fmt.Sprintf("task-%s", input.Task.ID))
	if _, err := os.Stat(worktreePath); err == nil {
		if err := runGitInRepo(input.RepoRoot, "worktree", "remove", "--force", worktreePath); err != nil {
			// Log warning but don't fail merge
			if input.Auditor != nil {
				_ = input.Auditor.Log(audit.Entry{
					TaskID: input.Task.ID,
					Role:   string(input.Task.Role),
					Event:  "worktree.remove.warning",
					Fields: []audit.Field{{Key: "error", Value: err.Error()}},
				})
			}
		}
	}

	// Step 7: Clean up task branch after successful merge
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

	// Step 8: Log successful transition to audit
	if input.Auditor != nil {
		_ = input.Auditor.LogTaskTransition(
			input.Task.ID,
			string(input.Task.Role),
			string(index.TaskStateTested),
			string(index.TaskStateMerged),
		)
	}

	return MergeFlowResult{
		Success:  true,
		NewState: index.TaskStateMerged,
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
				string(index.TaskStateMerged),
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

// fetchBranch fetches the named branch from the configured remote.
func fetchBranch(worktreePath string, remote string, branch string) error {
	if strings.TrimSpace(remote) == "" || strings.TrimSpace(branch) == "" {
		return errors.New("remote and branch are required")
	}
	if err := runGitInWorktree(worktreePath, "remote", "get-url", remote); err != nil {
		return fmt.Errorf("remote %s is not configured: %w", remote, err)
	}
	if err := runGitInWorktree(worktreePath, "fetch", remote, branch); err != nil {
		return fmt.Errorf("fetch %s/%s: %w", remote, branch, err)
	}
	return nil
}

// ensureCleanWorktree verifies the worktree has no uncommitted changes.
func ensureCleanWorktree(worktreePath string) error {
	if strings.TrimSpace(worktreePath) == "" {
		return errors.New("worktree path is required")
	}
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = worktreePath
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("check worktree status: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if strings.HasPrefix(path, "_governator/_local-state") {
			continue
		}
		return fmt.Errorf("worktree %s has uncommitted changes: %s", worktreePath, line)
	}
	return nil
}

// createMergeWorktree creates a temporary worktree on the main branch for safe merges.
func createMergeWorktree(repoRoot string, mainBranch string, taskID string) (string, func() error, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return "", nil, fmt.Errorf("repo root is required")
	}
	if strings.TrimSpace(mainBranch) == "" {
		return "", nil, fmt.Errorf("main branch is required")
	}
	if strings.TrimSpace(taskID) == "" {
		return "", nil, fmt.Errorf("task id is required")
	}

	mergeDir := filepath.Join(repoRoot, "_governator", "_local-state", "merge-worktrees")
	if err := os.MkdirAll(mergeDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create merge worktree dir %s: %w", mergeDir, err)
	}

	suffix := time.Now().UTC().Format("20060102-150405")
	worktreePath := filepath.Join(mergeDir, fmt.Sprintf("%s-%s", taskID, suffix))
	mergeBranch := fmt.Sprintf("governator-merge-%s-%s", taskID, suffix)
	if _, err := os.Stat(worktreePath); err == nil {
		return "", nil, fmt.Errorf("merge worktree path %s already exists", worktreePath)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", nil, fmt.Errorf("stat merge worktree path %s: %w", worktreePath, err)
	}

	if err := runGitInRepo(repoRoot, "fetch", "origin", mainBranch); err != nil {
		return "", nil, fmt.Errorf("fetch main branch %s: %w", mainBranch, err)
	}

	if err := runGitInRepo(repoRoot, "worktree", "add", "-b", mergeBranch, worktreePath, "origin/"+mainBranch); err != nil {
		return "", nil, fmt.Errorf("create merge worktree: %w", err)
	}

	cleanup := func() error {
		if err := runGitInRepo(repoRoot, "worktree", "remove", "--force", worktreePath); err != nil {
			return err
		}
		if err := runGitInRepo(repoRoot, "branch", "-D", mergeBranch); err != nil {
			return err
		}
		return nil
	}

	if err := ensureCleanWorktree(worktreePath); err != nil {
		_ = cleanup()
		return "", nil, err
	}

	return worktreePath, cleanup, nil
}

// getWorktreeCommit returns the current HEAD commit SHA of a worktree.
func getWorktreeCommit(worktreePath string) (string, error) {
	if strings.TrimSpace(worktreePath) == "" {
		return "", fmt.Errorf("worktree path is required")
	}

	cmd := exec.Command("git", "rev-parse", "HEAD")
	cmd.Dir = worktreePath

	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("get worktree commit: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
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
