// Tests for resume logic.
package run

import (
	"os"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/testrepos"
	"github.com/cmtonkinson/governator/internal/worktree"
)

// TestDetectResumeCandidatesHappyPath ensures blocked tasks with preserved worktrees are detected.
func TestDetectResumeCandidatesHappyPath(t *testing.T) {
	t.Parallel()
	repoRoot := setupTestRepo(t)

	// Create a test index with a blocked task
	idx := index.Index{
		Tasks: []index.Task{
			{
				ID:    "T-001",
				State: index.TaskStateBlocked,
				Attempts: index.AttemptCounters{
					Total:  1,
					Failed: 1,
				},
			},
		},
	}

	// Create a preserved worktree for the task
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}

	worktreePath, err := manager.WorktreePath("T-001")
	if err != nil {
		t.Fatalf("get worktree path: %v", err)
	}

	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create worktree directory: %v", err)
	}

	cfg := config.Config{}
	candidates, err := DetectResumeCandidates(repoRoot, idx, cfg)
	if err != nil {
		t.Fatalf("detect resume candidates: %v", err)
	}

	if len(candidates) != 1 {
		t.Fatalf("expected 1 candidate, got %d", len(candidates))
	}

	candidate := candidates[0]
	if candidate.Task.ID != "T-001" {
		t.Fatalf("candidate task ID = %q, want %q", candidate.Task.ID, "T-001")
	}
	if candidate.Attempt != 1 {
		t.Fatalf("candidate attempt = %d, want %d", candidate.Attempt, 1)
	}
}

// TestDetectResumeCandidatesSkipsNonBlockedTasks ensures only blocked tasks are considered.
func TestDetectResumeCandidatesSkipsNonBlockedTasks(t *testing.T) {
	t.Parallel()
	repoRoot := setupTestRepo(t)

	// Create a test index with tasks in various states
	idx := index.Index{
		Tasks: []index.Task{
			{
				ID:    "T-001",
				State: index.TaskStateOpen,
			},
			{
				ID:    "T-002",
				State: index.TaskStateWorked,
			},
			{
				ID:    "T-003",
				State: index.TaskStateDone,
			},
		},
	}

	cfg := config.Config{}
	candidates, err := DetectResumeCandidates(repoRoot, idx, cfg)
	if err != nil {
		t.Fatalf("detect resume candidates: %v", err)
	}

	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(candidates))
	}
}

// TestDetectResumeCandidatesSkipsTasksWithoutWorktrees ensures tasks without preserved worktrees are skipped.
func TestDetectResumeCandidatesSkipsTasksWithoutWorktrees(t *testing.T) {
	t.Parallel()
	repoRoot := setupTestRepo(t)

	// Create a test index with a blocked task but no preserved worktree
	idx := index.Index{
		Tasks: []index.Task{
			{
				ID:    "T-001",
				State: index.TaskStateBlocked,
				Attempts: index.AttemptCounters{
					Total:  1,
					Failed: 1,
				},
			},
		},
	}

	cfg := config.Config{}
	candidates, err := DetectResumeCandidates(repoRoot, idx, cfg)
	if err != nil {
		t.Fatalf("detect resume candidates: %v", err)
	}

	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(candidates))
	}
}

// TestProcessResumeCandidatesWithinRetryLimit ensures candidates within retry limits are resumed.
func TestProcessResumeCandidatesWithinRetryLimit(t *testing.T) {
	t.Parallel()

	candidates := []ResumeCandidate{
		{
			Task: index.Task{
				ID: "T-001",
				Attempts: index.AttemptCounters{
					Total:  1,
					Failed: 1,
				},
			},
			Attempt: 1,
		},
	}

	cfg := config.Config{
		Retries: config.RetriesConfig{
			MaxAttempts: 3,
		},
	}

	result := ProcessResumeCandidates(candidates, cfg)

	if len(result.Resumed) != 1 {
		t.Fatalf("expected 1 resumed candidate, got %d", len(result.Resumed))
	}
	if len(result.Blocked) != 0 {
		t.Fatalf("expected 0 blocked candidates, got %d", len(result.Blocked))
	}
}

// TestProcessResumeCandidatesExceedsRetryLimit ensures candidates exceeding retry limits are blocked.
func TestProcessResumeCandidatesExceedsRetryLimit(t *testing.T) {
	t.Parallel()

	candidates := []ResumeCandidate{
		{
			Task: index.Task{
				ID: "T-001",
				Attempts: index.AttemptCounters{
					Total:  3,
					Failed: 3,
				},
			},
			Attempt: 3,
		},
	}

	cfg := config.Config{
		Retries: config.RetriesConfig{
			MaxAttempts: 3,
		},
	}

	result := ProcessResumeCandidates(candidates, cfg)

	if len(result.Resumed) != 0 {
		t.Fatalf("expected 0 resumed candidates, got %d", len(result.Resumed))
	}
	if len(result.Blocked) != 1 {
		t.Fatalf("expected 1 blocked candidate, got %d", len(result.Blocked))
	}
}

// TestGetMaxAttemptsTaskSpecific ensures task-specific retry policies are used.
func TestGetMaxAttemptsTaskSpecific(t *testing.T) {
	t.Parallel()

	task := index.Task{
		Retries: index.RetryPolicy{
			MaxAttempts: 5,
		},
	}

	cfg := config.Config{
		Retries: config.RetriesConfig{
			MaxAttempts: 3,
		},
	}

	maxAttempts := getMaxAttempts(task, cfg)
	if maxAttempts != 5 {
		t.Fatalf("max attempts = %d, want %d", maxAttempts, 5)
	}
}

// TestGetMaxAttemptsGlobalConfig ensures global config is used when task-specific is not set.
func TestGetMaxAttemptsGlobalConfig(t *testing.T) {
	t.Parallel()

	task := index.Task{
		Retries: index.RetryPolicy{
			MaxAttempts: 0, // Not set
		},
	}

	cfg := config.Config{
		Retries: config.RetriesConfig{
			MaxAttempts: 3,
		},
	}

	maxAttempts := getMaxAttempts(task, cfg)
	if maxAttempts != 3 {
		t.Fatalf("max attempts = %d, want %d", maxAttempts, 3)
	}
}

// TestGetMaxAttemptsDefault ensures default is used when nothing is configured.
func TestGetMaxAttemptsDefault(t *testing.T) {
	t.Parallel()

	task := index.Task{
		Retries: index.RetryPolicy{
			MaxAttempts: 0, // Not set
		},
	}

	cfg := config.Config{
		Retries: config.RetriesConfig{
			MaxAttempts: 0, // Not set
		},
	}

	maxAttempts := getMaxAttempts(task, cfg)
	if maxAttempts != 3 {
		t.Fatalf("max attempts = %d, want %d", maxAttempts, 3)
	}
}

// TestPrepareTaskForResumeHappyPath ensures task is properly prepared for resume.
func TestPrepareTaskForResumeHappyPath(t *testing.T) {
	t.Parallel()

	idx := &index.Index{
		Tasks: []index.Task{
			{
				ID:    "T-001",
				State: index.TaskStateBlocked,
				Attempts: index.AttemptCounters{
					Total:  1,
					Failed: 1,
				},
			},
		},
	}

	err := PrepareTaskForResume(idx, "T-001", nil)
	if err != nil {
		t.Fatalf("prepare task for resume: %v", err)
	}

	// Check that attempt counter was incremented
	task := idx.Tasks[0]
	if task.Attempts.Total != 2 {
		t.Fatalf("task attempts total = %d, want %d", task.Attempts.Total, 2)
	}

	// Check that task state was transitioned to open
	if task.State != index.TaskStateOpen {
		t.Fatalf("task state = %q, want %q", task.State, index.TaskStateOpen)
	}
}

// TestPrepareTaskForResumeValidation ensures proper validation.
func TestPrepareTaskForResumeValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		idx     *index.Index
		taskID  string
		wantErr string
	}{
		{
			name:    "nil index",
			idx:     nil,
			taskID:  "T-001",
			wantErr: "index is required",
		},
		{
			name: "empty task id",
			idx: &index.Index{
				Tasks: []index.Task{},
			},
			taskID:  "",
			wantErr: "task id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := PrepareTaskForResume(tt.idx, tt.taskID, nil)
			if err == nil {
				t.Fatal("expected error")
			}
			if !containsString(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// containsString reports whether s contains substr.
func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}

// setupTestRepo creates a temporary git repository for testing.
func setupTestRepo(t *testing.T) string {
	t.Helper()
	return testrepos.New(t).Root
}
