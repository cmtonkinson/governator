// Package run provides resume logic for timed-out tasks.
package run

import (
	"fmt"
	"os"
	"strings"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/worktree"
)

// ResumeCandidate represents a task that may be eligible for resume.
type ResumeCandidate struct {
	Task         index.Task
	WorktreePath string
	Attempt      int
}

// ResumeResult captures the outcome of resume detection and processing.
type ResumeResult struct {
	Candidates []ResumeCandidate
	Resumed    []ResumeCandidate
	Blocked    []ResumeCandidate
}

// DetectResumeCandidates identifies tasks with preserved worktrees that can be resumed.
func DetectResumeCandidates(repoRoot string, idx index.Index, cfg config.Config) ([]ResumeCandidate, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, fmt.Errorf("repo root is required")
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("create worktree manager: %w", err)
	}

	var candidates []ResumeCandidate
	for _, task := range idx.Tasks {
		// Only consider blocked tasks for resume
		if task.State != index.TaskStateBlocked {
			continue
		}

		// Check if there's a preserved worktree for the current attempt
		currentAttempt := task.Attempts.Total
		if currentAttempt == 0 {
			currentAttempt = 1 // First attempt
		}

		worktreePath, err := manager.WorktreePath(task.ID, currentAttempt)
		if err != nil {
			continue // Skip tasks with invalid IDs
		}

		// Check if the worktree exists
		exists, err := pathExists(worktreePath)
		if err != nil {
			continue // Skip on error
		}
		if !exists {
			continue // No preserved worktree
		}

		candidates = append(candidates, ResumeCandidate{
			Task:         task,
			WorktreePath: worktreePath,
			Attempt:      currentAttempt,
		})
	}

	return candidates, nil
}

// ProcessResumeCandidates determines which candidates should be resumed vs blocked.
func ProcessResumeCandidates(candidates []ResumeCandidate, cfg config.Config) ResumeResult {
	result := ResumeResult{
		Candidates: candidates,
		Resumed:    make([]ResumeCandidate, 0),
		Blocked:    make([]ResumeCandidate, 0),
	}

	for _, candidate := range candidates {
		// Check if the task has exceeded retry limits
		maxAttempts := getMaxAttempts(candidate.Task, cfg)
		if candidate.Task.Attempts.Total >= maxAttempts {
			result.Blocked = append(result.Blocked, candidate)
		} else {
			result.Resumed = append(result.Resumed, candidate)
		}
	}

	return result
}

// getMaxAttempts returns the maximum attempts allowed for a task.
func getMaxAttempts(task index.Task, cfg config.Config) int {
	// Use task-specific retry policy if set
	if task.Retries.MaxAttempts > 0 {
		return task.Retries.MaxAttempts
	}
	// Fall back to global config
	if cfg.Retries.MaxAttempts > 0 {
		return cfg.Retries.MaxAttempts
	}
	// Default to 3 attempts if nothing is configured
	return 3
}

// pathExists reports whether the path exists on disk.
func pathExists(path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, fmt.Errorf("path is required")
	}
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat path %s: %w", path, err)
}
// PrepareTaskForResume increments the attempt counter and transitions the task to open.
func PrepareTaskForResume(idx *index.Index, taskID string, auditor index.TransitionAuditor) error {
	if idx == nil {
		return fmt.Errorf("index is required")
	}
	if strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("task id is required")
	}

	// Increment the attempt counter
	if err := index.IncrementTaskAttempt(idx, taskID); err != nil {
		return fmt.Errorf("increment task attempt: %w", err)
	}

	// Transition the task from blocked to open for retry
	if err := index.TransitionTaskStateWithAudit(idx, taskID, index.TaskStateOpen, auditor); err != nil {
		return fmt.Errorf("transition task to open: %w", err)
	}

	return nil
}

// BlockTaskWithRetryExceeded blocks a task that has exceeded its retry limit.
func BlockTaskWithRetryExceeded(idx *index.Index, taskID string, maxAttempts int, auditor index.TransitionAuditor) error {
	if idx == nil {
		return fmt.Errorf("index is required")
	}
	if strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("task id is required")
	}

	// The task should already be blocked, so we don't need to transition it
	// Just ensure it stays blocked by not changing its state
	return nil
}