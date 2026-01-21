// Package worker provides tests for worker result ingestion.
package worker

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
)

// TestIngestWorkerResultSuccess ensures successful ingestion with commit and marker.
func TestIngestWorkerResultSuccess(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Initialize git repo and create a commit
	setupGitRepo(t, workDir)
	createCommit(t, workDir, "test commit")

	// Create stage marker file
	markerPath := filepath.Join(workDir, "_governator", "_local_state", "worked.md")
	writeFile(t, markerPath, "Task completed successfully")

	input := IngestInput{
		TaskID:       "T-001",
		WorktreePath: workDir,
		Stage:        roles.StageWork,
		ExecResult: ExecResult{
			ExitCode: 0,
			Duration: time.Second,
		},
	}

	result, err := IngestWorkerResult(input)
	if err != nil {
		t.Fatalf("IngestWorkerResult failed: %v", err)
	}

	if !result.Success {
		t.Fatal("expected success")
	}
	if result.NewState != index.TaskStateWorked {
		t.Fatalf("new state = %q, want %q", result.NewState, index.TaskStateWorked)
	}
	if !result.HasCommit {
		t.Fatal("expected HasCommit to be true")
	}
	if !result.HasMarker {
		t.Fatal("expected HasMarker to be true")
	}
	if result.BlockReason != "" {
		t.Fatalf("expected empty block reason, got %q", result.BlockReason)
	}
}

// TestIngestWorkerResultMissingCommit ensures missing commit blocks the task.
func TestIngestWorkerResultMissingCommit(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Initialize git repo but don't create any commits
	setupGitRepo(t, workDir)

	// Create stage marker file
	markerPath := filepath.Join(workDir, "_governator", "_local_state", "worked.md")
	writeFile(t, markerPath, "Task completed successfully")

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := IngestInput{
		TaskID:       "T-002",
		WorktreePath: workDir,
		Stage:        roles.StageWork,
		ExecResult: ExecResult{
			ExitCode: 0,
			Duration: time.Second,
		},
		Warn: warn,
	}

	result, err := IngestWorkerResult(input)
	if err != nil {
		t.Fatalf("IngestWorkerResult failed: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}
	if result.NewState != index.TaskStateBlocked {
		t.Fatalf("new state = %q, want %q", result.NewState, index.TaskStateBlocked)
	}
	if result.HasCommit {
		t.Fatal("expected HasCommit to be false")
	}
	if !result.HasMarker {
		t.Fatal("expected HasMarker to be true")
	}
	if !strings.Contains(result.BlockReason, "missing commit") {
		t.Fatalf("block reason = %q, want missing commit message", result.BlockReason)
	}

	// Check that warning was emitted
	if len(warnings) == 0 {
		t.Fatal("expected warning")
	}
	if !strings.Contains(warnings[0], "blocked") {
		t.Fatalf("warning = %q, want blocked message", warnings[0])
	}
}

// TestIngestWorkerResultMissingMarker ensures missing marker blocks the task.
func TestIngestWorkerResultMissingMarker(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Initialize git repo and create a commit
	setupGitRepo(t, workDir)
	createCommit(t, workDir, "test commit")

	// Don't create stage marker file

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := IngestInput{
		TaskID:       "T-003",
		WorktreePath: workDir,
		Stage:        roles.StageWork,
		ExecResult: ExecResult{
			ExitCode: 0,
			Duration: time.Second,
		},
		Warn: warn,
	}

	result, err := IngestWorkerResult(input)
	if err != nil {
		t.Fatalf("IngestWorkerResult failed: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}
	if result.NewState != index.TaskStateBlocked {
		t.Fatalf("new state = %q, want %q", result.NewState, index.TaskStateBlocked)
	}
	if !result.HasCommit {
		t.Fatal("expected HasCommit to be true")
	}
	if result.HasMarker {
		t.Fatal("expected HasMarker to be false")
	}
	if !strings.Contains(result.BlockReason, "missing worked.md marker") {
		t.Fatalf("block reason = %q, want missing marker message", result.BlockReason)
	}

	// Check that warning was emitted
	if len(warnings) == 0 {
		t.Fatal("expected warning")
	}
	if !strings.Contains(warnings[0], "blocked") {
		t.Fatalf("warning = %q, want blocked message", warnings[0])
	}
}

// TestIngestWorkerResultMissingBoth ensures missing commit and marker blocks the task.
func TestIngestWorkerResultMissingBoth(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	// Initialize git repo but don't create commits or marker
	setupGitRepo(t, workDir)

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := IngestInput{
		TaskID:       "T-004",
		WorktreePath: workDir,
		Stage:        roles.StageWork,
		ExecResult: ExecResult{
			ExitCode: 0,
			Duration: time.Second,
		},
		Warn: warn,
	}

	result, err := IngestWorkerResult(input)
	if err != nil {
		t.Fatalf("IngestWorkerResult failed: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}
	if result.NewState != index.TaskStateBlocked {
		t.Fatalf("new state = %q, want %q", result.NewState, index.TaskStateBlocked)
	}
	if result.HasCommit {
		t.Fatal("expected HasCommit to be false")
	}
	if result.HasMarker {
		t.Fatal("expected HasMarker to be false")
	}
	if !strings.Contains(result.BlockReason, "missing both commit and worked.md marker") {
		t.Fatalf("block reason = %q, want missing both message", result.BlockReason)
	}
}

// TestIngestWorkerResultExecutionFailure ensures execution failures block the task.
func TestIngestWorkerResultExecutionFailure(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := IngestInput{
		TaskID:       "T-005",
		WorktreePath: workDir,
		Stage:        roles.StageWork,
		ExecResult: ExecResult{
			ExitCode: 1,
			Duration: time.Second,
			Error:    fmt.Errorf("worker process exited with code 1"),
		},
		Warn: warn,
	}

	result, err := IngestWorkerResult(input)
	if err != nil {
		t.Fatalf("IngestWorkerResult failed: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}
	if result.NewState != index.TaskStateBlocked {
		t.Fatalf("new state = %q, want %q", result.NewState, index.TaskStateBlocked)
	}
	if !strings.Contains(result.BlockReason, "worker execution failed") {
		t.Fatalf("block reason = %q, want execution failed message", result.BlockReason)
	}

	// Check that warning was emitted
	if len(warnings) == 0 {
		t.Fatal("expected warning")
	}
	if !strings.Contains(warnings[0], "blocked") {
		t.Fatalf("warning = %q, want blocked message", warnings[0])
	}
}

// TestIngestWorkerResultTimeout ensures timeouts block the task with specific message.
func TestIngestWorkerResultTimeout(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	var warnings []string
	warn := func(msg string) {
		warnings = append(warnings, msg)
	}

	input := IngestInput{
		TaskID:       "T-006",
		WorktreePath: workDir,
		Stage:        roles.StageWork,
		ExecResult: ExecResult{
			ExitCode: -1,
			TimedOut: true,
			Duration: 30 * time.Second,
			Error:    fmt.Errorf("worker process timed out after 30 seconds"),
		},
		Warn: warn,
	}

	result, err := IngestWorkerResult(input)
	if err != nil {
		t.Fatalf("IngestWorkerResult failed: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}
	if result.NewState != index.TaskStateBlocked {
		t.Fatalf("new state = %q, want %q", result.NewState, index.TaskStateBlocked)
	}
	if !strings.Contains(result.BlockReason, "timed out") {
		t.Fatalf("block reason = %q, want timeout message", result.BlockReason)
	}
}

// TestIngestWorkerResultAllStages ensures all stages map to correct success states.
func TestIngestWorkerResultAllStages(t *testing.T) {
	t.Parallel()

	stages := []struct {
		stage        roles.Stage
		successState index.TaskState
		markerFile   string
	}{
		{roles.StageWork, index.TaskStateWorked, "worked.md"},
		{roles.StageTest, index.TaskStateTested, "tested.md"},
		{roles.StageReview, index.TaskStateDone, "reviewed.md"},
		{roles.StageResolve, index.TaskStateResolved, "resolved.md"},
	}

	for _, s := range stages {
		t.Run(string(s.stage), func(t *testing.T) {
			workDir := t.TempDir()

			// Initialize git repo and create a commit
			setupGitRepo(t, workDir)
			createCommit(t, workDir, "test commit")

			// Create stage marker file
			markerPath := filepath.Join(workDir, "_governator", "_local_state", s.markerFile)
			writeFile(t, markerPath, "Stage completed successfully")

			input := IngestInput{
				TaskID:       "T-" + string(s.stage),
				WorktreePath: workDir,
				Stage:        s.stage,
				ExecResult: ExecResult{
					ExitCode: 0,
					Duration: time.Second,
				},
			}

			result, err := IngestWorkerResult(input)
			if err != nil {
				t.Fatalf("IngestWorkerResult failed: %v", err)
			}

			if !result.Success {
				t.Fatal("expected success")
			}
			if result.NewState != s.successState {
				t.Fatalf("new state = %q, want %q", result.NewState, s.successState)
			}
		})
	}
}

// TestIngestWorkerResultValidation ensures input validation works correctly.
func TestIngestWorkerResultValidation(t *testing.T) {
	t.Parallel()
	workDir := t.TempDir()

	tests := []struct {
		name    string
		input   IngestInput
		wantErr string
	}{
		{
			name: "empty task id",
			input: IngestInput{
				TaskID:       "",
				WorktreePath: workDir,
				Stage:        roles.StageWork,
			},
			wantErr: "task id is required",
		},
		{
			name: "empty worktree path",
			input: IngestInput{
				TaskID:       "T-001",
				WorktreePath: "",
				Stage:        roles.StageWork,
			},
			wantErr: "worktree path is required",
		},
		{
			name: "invalid stage",
			input: IngestInput{
				TaskID:       "T-001",
				WorktreePath: workDir,
				Stage:        "invalid",
			},
			wantErr: "invalid stage",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := IngestWorkerResult(tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestIngestWorkerResultNonexistentWorktree ensures nonexistent worktree is handled.
func TestIngestWorkerResultNonexistentWorktree(t *testing.T) {
	t.Parallel()
	workDir := filepath.Join(t.TempDir(), "nonexistent")

	input := IngestInput{
		TaskID:       "T-007",
		WorktreePath: workDir,
		Stage:        roles.StageWork,
		ExecResult: ExecResult{
			ExitCode: 0,
			Duration: time.Second,
		},
	}

	result, err := IngestWorkerResult(input)
	if err != nil {
		t.Fatalf("IngestWorkerResult failed: %v", err)
	}

	if result.Success {
		t.Fatal("expected failure")
	}
	if result.HasCommit {
		t.Fatal("expected HasCommit to be false for nonexistent worktree")
	}
	if result.HasMarker {
		t.Fatal("expected HasMarker to be false for nonexistent worktree")
	}
}

// setupGitRepo initializes a git repository in the given directory.
func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "user.email", "test@example.com")
}

// createCommit creates a commit in the git repository.
func createCommit(t *testing.T, dir string, message string) {
	t.Helper()
	// Create a file to commit
	testFile := filepath.Join(dir, "test.txt")
	writeFile(t, testFile, "test content")
	runGit(t, dir, "add", "test.txt")
	runGit(t, dir, "commit", "-m", message)
}

// runGit executes a git command in the provided directory.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := runGitWithDir(dir, args...)
	if err != nil {
		t.Fatalf("git %s failed: %v", strings.Join(args, " "), err)
	}
	return output
}
