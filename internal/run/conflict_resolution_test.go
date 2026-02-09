// Tests for conflict resolution stage functionality.
package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/scheduler"
	"github.com/cmtonkinson/governator/internal/worker"
)

func TestExecuteConflictResolutionAgent_Success(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create task file
	taskPath := filepath.Join(tempDir, "task-01.md")
	if err := os.WriteFile(taskPath, []byte("# Task 1\nTest task content"), 0644); err != nil {
		t.Fatalf("failed to create task file: %v", err)
	}

	// Create roles directory with a test role
	rolesDir := filepath.Join(tempDir, "_governator", "roles")
	if err := os.MkdirAll(rolesDir, 0755); err != nil {
		t.Fatalf("failed to create roles dir: %v", err)
	}
	rolePath := filepath.Join(rolesDir, "default.md")
	if err := os.WriteFile(rolePath, []byte("# Generalist Role\nGeneral purpose role"), 0644); err != nil {
		t.Fatalf("failed to create role file: %v", err)
	}

	task := index.Task{
		ID:       "task-01",
		Kind:     index.TaskKindExecution,
		State:    index.TaskStateConflict,
		Role:     "default",
		Title:    "Test Task 1",
		Path:     "task-01.md",
		Attempts: index.AttemptCounters{Total: 1},
	}

	cfg := config.Config{
		Concurrency: config.ConcurrencyConfig{
			Global:      10,
			DefaultRole: 5,
			Roles:       map[string]int{"default": 3},
		},
	}

	var stdout strings.Builder
	var stderr strings.Builder
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// This will fail at the worker execution stage since we don't have a real worker setup,
	// but we can verify the role selection and staging works
	_, _, err := ExecuteConflictResolutionAgent(tempDir, tempDir, task, cfg, index.Index{Tasks: []index.Task{task}}, nil, opts)

	// We expect this to fail at the worker execution stage, which is fine for this test
	if err == nil {
		t.Fatal("expected error due to missing worker setup, got nil")
	}

	// Verify the error is from worker execution, not role selection or staging
	if !strings.Contains(err.Error(), "execute conflict resolution worker") &&
		!strings.Contains(err.Error(), "stage conflict resolution environment") {
		t.Errorf("unexpected error type: %v", err)
	}
}

func TestConfigureConflictResolutionStageInput(t *testing.T) {
	tempDir := t.TempDir()
	task := index.Task{
		ID:    "010-task",
		Title: "Resolve merge conflict",
		Path:  "_governator/tasks/010-task.md",
	}
	stageInput := newWorkerStageInput(
		tempDir,
		tempDir,
		task,
		"resolve",
		"default",
		1,
		config.Defaults(),
		nil,
	)

	if err := configureConflictResolutionStageInput(tempDir, task, &stageInput); err != nil {
		t.Fatalf("configureConflictResolutionStageInput: %v", err)
	}
	if stageInput.TaskPromptPath != conflictResolutionPromptPath {
		t.Fatalf("TaskPromptPath = %q, want %q", stageInput.TaskPromptPath, conflictResolutionPromptPath)
	}
	if got := stageInput.ExtraEnv["GOVERNATOR_CONFLICT_BRANCH"]; got != TaskBranchName(task) {
		t.Fatalf("GOVERNATOR_CONFLICT_BRANCH = %q, want %q", got, TaskBranchName(task))
	}
	if got := stageInput.ExtraEnv["GOVERNATOR_CONFLICT_TASK_PATH"]; got != task.Path {
		t.Fatalf("GOVERNATOR_CONFLICT_TASK_PATH = %q, want %q", got, task.Path)
	}
	if len(stageInput.ExtraPromptPath) != 1 {
		t.Fatalf("ExtraPromptPath len = %d, want 1", len(stageInput.ExtraPromptPath))
	}

	contextPrompt := filepath.Join(tempDir, filepath.FromSlash(stageInput.ExtraPromptPath[0]))
	content, err := os.ReadFile(contextPrompt)
	if err != nil {
		t.Fatalf("read conflict context prompt: %v", err)
	}
	got := string(content)
	if !strings.Contains(got, TaskBranchName(task)) {
		t.Fatalf("context prompt missing branch name: %q", got)
	}
	if !strings.Contains(got, task.Path) {
		t.Fatalf("context prompt missing task path: %q", got)
	}
}

func TestSelectRoleForConflictResolution_Success(t *testing.T) {
	task := index.Task{
		ID:    "task-01",
		Kind:  index.TaskKindExecution,
		State: index.TaskStateConflict,
		Role:  "architect",
		Title: "Test Task 1",
		Path:  "task-01.md",
	}

	result := SelectRoleForConflictResolution(task)
	if result.Role != "architect" {
		t.Fatalf("role = %q, want %q", result.Role, "architect")
	}
	if !result.Fallback {
		t.Fatalf("expected fallback=true for deterministic selection")
	}
}

func TestSelectRoleForConflictResolution_DefaultFallback(t *testing.T) {
	task := index.Task{
		ID:    "task-01",
		Kind:  index.TaskKindExecution,
		State: index.TaskStateConflict,
		Role:  "",
	}
	result := SelectRoleForConflictResolution(task)
	if result.Role != "default" {
		t.Fatalf("role = %q, want %q", result.Role, "default")
	}
}

func TestExecuteMergeStage_Success(t *testing.T) {
	// Create test index with resolved tasks
	idx := &index.Index{
		Tasks: []index.Task{
			{
				ID:       "task-01",
				Kind:     index.TaskKindExecution,
				State:    index.TaskStateResolved,
				Role:     "default",
				Title:    "Test Task 1",
				Attempts: index.AttemptCounters{Total: 1},
			},
		},
	}

	cfg := config.Config{}
	auditor := &mockTransitionAuditor{}
	caps := scheduler.RoleCapsFromConfig(cfg)

	var stdout strings.Builder
	var stderr strings.Builder
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// This will fail because we don't have actual worktrees set up,
	// but we can verify the function processes the resolved tasks
	result, err := ExecuteMergeStage("/tmp", idx, cfg, caps, nil, auditor, nil, opts)
	if err != nil {
		t.Fatalf("ExecuteMergeStage failed: %v", err)
	}

	// Should have processed 1 resolved task
	if result.TasksProcessed != 1 {
		t.Errorf("expected 1 task processed, got %d", result.TasksProcessed)
	}

	// Check that some output was written to stderr (warnings about worktree failures)
	stderrOutput := stderr.String()
	if stderrOutput == "" {
		t.Errorf("expected some warnings in stderr, got empty output")
	}
}

func TestUpdateTaskStateFromMerge_Success(t *testing.T) {
	// Create test index
	idx := &index.Index{
		Tasks: []index.Task{
			{
				ID:    "task-01",
				Kind:  index.TaskKindExecution,
				State: index.TaskStateMergeable,
				Role:  "default",
			},
		},
	}

	auditor := &mockTransitionAuditor{}

	// Test successful merge (mergeable -> merged)
	result := worker.IngestResult{
		Success:  true,
		NewState: index.TaskStateMerged,
	}

	err := UpdateTaskStateFromMerge(idx, "task-01", result, auditor)
	if err != nil {
		t.Fatalf("UpdateTaskStateFromMerge failed: %v", err)
	}

	// Verify task state was updated
	if idx.Tasks[0].State != index.TaskStateMerged {
		t.Errorf("expected task state %s, got %s", index.TaskStateMerged, idx.Tasks[0].State)
	}

	// Verify audit log was called
	if len(auditor.transitions) != 1 {
		t.Errorf("expected 1 audit transition, got %d", len(auditor.transitions))
	}

	transition := auditor.transitions[0]
	if transition.from != string(index.TaskStateMergeable) {
		t.Errorf("expected from state %s, got %s", index.TaskStateMergeable, transition.from)
	}
	if transition.to != string(index.TaskStateMerged) {
		t.Errorf("expected to state %s, got %s", index.TaskStateMerged, transition.to)
	}
}

func TestUpdateTaskStateFromMerge_Failure(t *testing.T) {
	// Create test index
	idx := &index.Index{
		Tasks: []index.Task{
			{
				ID:    "task-01",
				Kind:  index.TaskKindExecution,
				State: index.TaskStateMergeable,
				Role:  "default",
			},
		},
	}

	auditor := &mockTransitionAuditor{}

	// Test failed merge (resolved -> conflict)
	result := worker.IngestResult{
		Success:     false,
		NewState:    index.TaskStateConflict,
		BlockReason: "merge conflict detected",
	}

	err := UpdateTaskStateFromMerge(idx, "task-01", result, auditor)
	if err != nil {
		t.Fatalf("UpdateTaskStateFromMerge failed: %v", err)
	}

	// Verify task state was updated to conflict (not the NewState from result)
	if idx.Tasks[0].State != index.TaskStateConflict {
		t.Errorf("expected task state %s, got %s", index.TaskStateConflict, idx.Tasks[0].State)
	}

	// Verify audit log was called
	if len(auditor.transitions) != 1 {
		t.Errorf("expected 1 audit transition, got %d", len(auditor.transitions))
	}

	transition := auditor.transitions[0]
	if transition.from != string(index.TaskStateMergeable) {
		t.Errorf("expected from state %s, got %s", index.TaskStateMergeable, transition.from)
	}
	if transition.to != string(index.TaskStateConflict) {
		t.Errorf("expected to state %s, got %s", index.TaskStateConflict, transition.to)
	}
}
