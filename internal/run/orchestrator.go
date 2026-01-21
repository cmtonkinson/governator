// Package run provides the main orchestration logic for the run command.
package run

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

const (
	// indexFilePath is the relative path to the task index file.
	indexFilePath = "_governator/task-index.json"
)

// Options defines the configuration for a run execution.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Result captures the outcome of a run execution.
type Result struct {
	ResumedTasks []string
	BlockedTasks []string
	Message      string
}

// Run executes the main run orchestration including resume logic.
func Run(repoRoot string, opts Options) (Result, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Result{}, fmt.Errorf("repo root is required")
	}
	if opts.Stdout == nil || opts.Stderr == nil {
		return Result{}, fmt.Errorf("stdout and stderr are required")
	}

	// Load configuration
	cfg, err := config.Load(repoRoot, nil, nil)
	if err != nil {
		return Result{}, fmt.Errorf("load config: %w", err)
	}

	// Load task index
	indexPath := filepath.Join(repoRoot, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		return Result{}, fmt.Errorf("load task index: %w", err)
	}

	// Check for planning drift
	if err := CheckPlanningDrift(repoRoot, idx.Digests); err != nil {
		return Result{}, err
	}

	// Set up audit logging
	auditor, err := audit.NewLogger(repoRoot, opts.Stderr)
	if err != nil {
		return Result{}, fmt.Errorf("create audit logger: %w", err)
	}

	// Detect resume candidates
	candidates, err := DetectResumeCandidates(repoRoot, idx, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("detect resume candidates: %w", err)
	}

	// Process resume candidates
	resumeResult := ProcessResumeCandidates(candidates, cfg)

	var resumedTasks []string
	var blockedTasks []string

	// Process tasks to be resumed
	for _, candidate := range resumeResult.Resumed {
		if err := PrepareTaskForResume(&idx, candidate.Task.ID, auditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to prepare task %s for resume: %v\n", candidate.Task.ID, err)
			continue
		}
		resumedTasks = append(resumedTasks, candidate.Task.ID)
		fmt.Fprintf(opts.Stdout, "Resuming task %s (attempt %d)\n", candidate.Task.ID, candidate.Task.Attempts.Total+1)
	}

	// Process tasks that exceeded retry limits
	for _, candidate := range resumeResult.Blocked {
		maxAttempts := getMaxAttempts(candidate.Task, cfg)
		if err := BlockTaskWithRetryExceeded(&idx, candidate.Task.ID, maxAttempts, auditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to block task %s: %v\n", candidate.Task.ID, err)
			continue
		}
		blockedTasks = append(blockedTasks, candidate.Task.ID)
		fmt.Fprintf(opts.Stdout, "Task %s blocked: exceeded retry limit (%d attempts)\n", candidate.Task.ID, maxAttempts)
	}

	// Save updated index
	if len(resumedTasks) > 0 || len(blockedTasks) > 0 {
		if err := index.Save(indexPath, idx); err != nil {
			return Result{}, fmt.Errorf("save task index: %w", err)
		}
	}

	// Build result message
	var message strings.Builder
	if len(resumedTasks) > 0 {
		message.WriteString(fmt.Sprintf("Resumed %d task(s)", len(resumedTasks)))
	}
	if len(blockedTasks) > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		message.WriteString(fmt.Sprintf("blocked %d task(s) due to retry limit", len(blockedTasks)))
	}
	if message.Len() == 0 {
		message.WriteString("No tasks to resume")
	}

	return Result{
		ResumedTasks: resumedTasks,
		BlockedTasks: blockedTasks,
		Message:      message.String(),
	}, nil
}