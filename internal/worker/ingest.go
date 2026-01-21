// Package worker provides worker result ingestion for updating task state.
package worker

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
)

// IngestInput defines the inputs required for worker result ingestion.
type IngestInput struct {
	TaskID       string
	WorktreePath string
	Stage        roles.Stage
	ExecResult   ExecResult
	Warn         func(string)
}

// IngestResult captures the worker result ingestion outcome.
type IngestResult struct {
	Success      bool
	NewState     index.TaskState
	BlockReason  string
	TimedOut     bool // TimedOut reports whether the worker execution timed out.
	HasCommit    bool
	HasMarker    bool
	MarkerPath   string
	MarkerExists bool
}

// IngestWorkerResult processes worker execution results and determines task state changes.
func IngestWorkerResult(input IngestInput) (IngestResult, error) {
	if strings.TrimSpace(input.TaskID) == "" {
		return IngestResult{}, errors.New("task id is required")
	}
	if strings.TrimSpace(input.WorktreePath) == "" {
		return IngestResult{}, errors.New("worktree path is required")
	}
	if !input.Stage.Valid() {
		return IngestResult{}, fmt.Errorf("invalid stage %q", input.Stage)
	}

	// Check if worker execution failed
	if input.ExecResult.Error != nil {
		blockReason := fmt.Sprintf("worker execution failed: %s", input.ExecResult.Error.Error())
		if input.ExecResult.TimedOut {
			blockReason = fmt.Sprintf("worker execution timed out after %s", input.ExecResult.Duration)
		}
		emitWarning(input.Warn, fmt.Sprintf("task %s blocked: %s", input.TaskID, blockReason))
		return IngestResult{
			Success:     false,
			NewState:    index.TaskStateBlocked,
			BlockReason: blockReason,
			TimedOut:    input.ExecResult.TimedOut,
		}, nil
	}

	// Check for commit on task branch
	hasCommit, err := checkForCommit(input.WorktreePath)
	if err != nil {
		return IngestResult{}, fmt.Errorf("check for commit: %w", err)
	}

	// Check for stage marker file
	markerPath := filepath.Join(input.WorktreePath, localStateDirName, markerFileName(input.Stage))
	hasMarker, err := checkForMarkerFile(markerPath)
	if err != nil {
		return IngestResult{}, fmt.Errorf("check for marker file: %w", err)
	}

	result := IngestResult{
		HasCommit:    hasCommit,
		HasMarker:    hasMarker,
		MarkerPath:   repoRelativePath(input.WorktreePath, markerPath),
		MarkerExists: hasMarker,
	}

	// Determine success based on both commit and marker presence
	if hasCommit && hasMarker {
		result.Success = true
		result.NewState = stageToSuccessState(input.Stage)
	} else {
		result.Success = false
		result.NewState = index.TaskStateBlocked
		result.BlockReason = buildBlockReason(hasCommit, hasMarker, input.Stage)
		emitWarning(input.Warn, fmt.Sprintf("task %s blocked: %s", input.TaskID, result.BlockReason))
	}

	return result, nil
}

// checkForCommit verifies that there is at least one commit on the current branch.
func checkForCommit(worktreePath string) (bool, error) {
	if strings.TrimSpace(worktreePath) == "" {
		return false, errors.New("worktree path is required")
	}

	// Check if the worktree directory exists
	info, err := os.Stat(worktreePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat worktree path %s: %w", worktreePath, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("worktree path %s is not a directory", worktreePath)
	}

	// Use git to check if there are any commits on the current branch
	// This checks if HEAD exists and points to a valid commit
	output, err := runGitWithDir(worktreePath, "rev-parse", "--verify", "HEAD")
	if err != nil {
		// If HEAD doesn't exist or is invalid, there are no commits
		return false, nil
	}

	// If we got output without error, HEAD exists and points to a commit
	return strings.TrimSpace(output) != "", nil
}

// checkForMarkerFile verifies that the expected stage marker file exists.
func checkForMarkerFile(markerPath string) (bool, error) {
	if strings.TrimSpace(markerPath) == "" {
		return false, errors.New("marker path is required")
	}

	info, err := os.Stat(markerPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat marker file %s: %w", markerPath, err)
	}

	return info.Mode().IsRegular(), nil
}

// stageToSuccessState maps a stage to its corresponding success state.
func stageToSuccessState(stage roles.Stage) index.TaskState {
	switch stage {
	case roles.StageWork:
		return index.TaskStateWorked
	case roles.StageTest:
		return index.TaskStateTested
	case roles.StageReview:
		return index.TaskStateDone
	case roles.StageResolve:
		return index.TaskStateResolved
	default:
		return index.TaskStateBlocked
	}
}

// buildBlockReason constructs a descriptive reason for why a task is blocked.
func buildBlockReason(hasCommit bool, hasMarker bool, stage roles.Stage) string {
	if !hasCommit && !hasMarker {
		return fmt.Sprintf("missing both commit and %s marker file", markerFileName(stage))
	}
	if !hasCommit {
		return "missing commit on task branch"
	}
	if !hasMarker {
		return fmt.Sprintf("missing %s marker file", markerFileName(stage))
	}
	return "unknown blocking condition"
}

// runGitWithDir runs a git command in the provided directory.
func runGitWithDir(dir string, args ...string) (string, error) {
	if strings.TrimSpace(dir) == "" {
		return "", errors.New("git directory is required")
	}
	if len(args) == 0 {
		return "", errors.New("git arguments are required")
	}
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
