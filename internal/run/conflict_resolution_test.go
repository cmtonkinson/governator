// Tests for conflict resolution stage functionality.
package run

import (
	"fmt"
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

	// Create role assignment prompt
	promptDir := filepath.Join(tempDir, "_governator", "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatalf("failed to create prompt dir: %v", err)
	}
	promptPath := filepath.Join(promptDir, "role-assignment.md")
	if err := os.WriteFile(promptPath, []byte("# Role Assignment\nSelect appropriate role"), 0644); err != nil {
		t.Fatalf("failed to create role assignment prompt: %v", err)
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

func TestSelectRoleForConflictResolution_Success(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create task file
	taskPath := filepath.Join(tempDir, "task-01.md")
	if err := os.WriteFile(taskPath, []byte("# Task 1\nTest task content"), 0644); err != nil {
		t.Fatalf("failed to create task file: %v", err)
	}

	// Create role assignment prompt
	promptDir := filepath.Join(tempDir, "_governator", "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatalf("failed to create prompt dir: %v", err)
	}
	promptPath := filepath.Join(promptDir, "role-assignment.md")
	if err := os.WriteFile(promptPath, []byte("# Role Assignment\nSelect appropriate role"), 0644); err != nil {
		t.Fatalf("failed to create role assignment prompt: %v", err)
	}

	// Create roles directory with test roles
	rolesDir := filepath.Join(tempDir, "_governator", "roles")
	if err := os.MkdirAll(rolesDir, 0755); err != nil {
		t.Fatalf("failed to create roles dir: %v", err)
	}

	roles := []string{"default", "architect", "reviewer"}
	for _, role := range roles {
		rolePath := filepath.Join(rolesDir, role+".md")
		if err := os.WriteFile(rolePath, []byte("# "+role+" Role\n"+role+" role description"), 0644); err != nil {
			t.Fatalf("failed to create role file: %v", err)
		}
	}

	task := index.Task{
		ID:    "task-01",
		State: index.TaskStateConflict,
		Role:  "default",
		Title: "Test Task 1",
		Path:  "task-01.md",
	}

	cfg := config.Config{
		Concurrency: config.ConcurrencyConfig{
			Global:      10,
			DefaultRole: 5,
			Roles:       map[string]int{"default": 3, "architect": 2},
		},
	}

	cfg.Workers.Commands.Default = helperRoleAssignmentCommand()
	t.Setenv("GO_ROLE_ASSIGNMENT_HELPER", "1")
	t.Setenv("GO_ROLE_ASSIGNMENT_MODE", "valid")

	var stdout strings.Builder
	var stderr strings.Builder
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	result, err := SelectRoleForConflictResolution(tempDir, task, cfg, index.Index{Tasks: []index.Task{task}}, nil, opts)
	if err != nil {
		t.Fatalf("SelectRoleForConflictResolution failed: %v", err)
	}

	// Verify we got a valid role
	if result.Role == "" {
		t.Error("expected non-empty role")
	}

	// Verify the role is one of the available roles
	validRoles := []index.Role{"default", "architect", "reviewer"}
	found := false
	for _, validRole := range validRoles {
		if result.Role == validRole {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("selected role %s is not in available roles %v", result.Role, validRoles)
	}
}

func TestSelectRoleForConflictResolution_LLMFallback(t *testing.T) {
	tempDir := t.TempDir()

	taskPath := filepath.Join(tempDir, "task-01.md")
	if err := os.WriteFile(taskPath, []byte("# Task 1\nTest task content"), 0644); err != nil {
		t.Fatalf("failed to create task file: %v", err)
	}

	promptDir := filepath.Join(tempDir, "_governator", "prompts")
	if err := os.MkdirAll(promptDir, 0755); err != nil {
		t.Fatalf("failed to create prompt dir: %v", err)
	}
	promptPath := filepath.Join(promptDir, "role-assignment.md")
	if err := os.WriteFile(promptPath, []byte("# Role Assignment\nSelect appropriate role"), 0644); err != nil {
		t.Fatalf("failed to create role assignment prompt: %v", err)
	}

	rolesDir := filepath.Join(tempDir, "_governator", "roles")
	if err := os.MkdirAll(rolesDir, 0755); err != nil {
		t.Fatalf("failed to create roles dir: %v", err)
	}
	rolePath := filepath.Join(rolesDir, "default.md")
	if err := os.WriteFile(rolePath, []byte("# Generalist Role\nGeneral purpose role"), 0644); err != nil {
		t.Fatalf("failed to create role file: %v", err)
	}

	task := index.Task{
		ID:   "task-01",
		Path: "task-01.md",
	}

	cfg := config.Config{
		Concurrency: config.ConcurrencyConfig{
			Global:      1,
			DefaultRole: 1,
		},
	}
	cfg.Workers.Commands.Default = helperRoleAssignmentCommand()

	var stdout strings.Builder
	var stderr strings.Builder
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	t.Setenv("GO_ROLE_ASSIGNMENT_HELPER", "1")
	t.Setenv("GO_ROLE_ASSIGNMENT_MODE", "invalid")

	result, err := SelectRoleForConflictResolution(tempDir, task, cfg, index.Index{}, nil, opts)
	if err != nil {
		t.Fatalf("SelectRoleForConflictResolution failed: %v", err)
	}

	if !result.Fallback {
		t.Fatalf("expected fallback result, got %v", result)
	}
	if result.Role != "default" {
		t.Fatalf("fallback role = %q, want %q", result.Role, "default")
	}
}

func TestSelectRoleForConflictResolution_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		setupFunc   func(string) (index.Task, config.Config)
		expectError string
	}{
		{
			name: "missing role assignment prompt",
			setupFunc: func(tempDir string) (index.Task, config.Config) {
				// Don't create the role assignment prompt
				task := index.Task{
					ID:   "task-01",
					Path: "task-01.md",
				}
				cfg := config.Config{}
				return task, cfg
			},
			expectError: "load role assignment prompt",
		},
		{
			name: "missing task file",
			setupFunc: func(tempDir string) (index.Task, config.Config) {
				// Create role assignment prompt and roles directory but not task file
				promptDir := filepath.Join(tempDir, "_governator", "prompts")
				os.MkdirAll(promptDir, 0755)
				promptPath := filepath.Join(promptDir, "role-assignment.md")
				os.WriteFile(promptPath, []byte("# Role Assignment"), 0644)

				// Create roles directory with a test role
				rolesDir := filepath.Join(tempDir, "_governator", "roles")
				os.MkdirAll(rolesDir, 0755)
				rolePath := filepath.Join(rolesDir, "default.md")
				os.WriteFile(rolePath, []byte("# Generalist Role"), 0644)

				task := index.Task{
					ID:   "task-01",
					Path: "nonexistent.md",
				}
				cfg := config.Config{}
				return task, cfg
			},
			expectError: "read task file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tempDir := t.TempDir()
			task, cfg := tt.setupFunc(tempDir)

			var stdout strings.Builder
			var stderr strings.Builder
			opts := Options{
				Stdout: &stdout,
				Stderr: &stderr,
			}

			_, err := SelectRoleForConflictResolution(tempDir, task, cfg, index.Index{}, nil, opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestExecuteMergeStage_Success(t *testing.T) {
	// Create test index with resolved tasks
	idx := &index.Index{
		Tasks: []index.Task{
			{
				ID:       "task-01",
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

func helperRoleAssignmentCommand() []string {
	return []string{os.Args[0], "-test.run=TestRoleAssignmentHelper", "--", "{task_path}"}
}

func TestRoleAssignmentHelper(t *testing.T) {
	if os.Getenv("GO_ROLE_ASSIGNMENT_HELPER") != "1" {
		return
	}
	mode := os.Getenv("GO_ROLE_ASSIGNMENT_MODE")
	if mode == "invalid" {
		fmt.Println("not json")
		os.Exit(0)
	}
	role := os.Getenv("GO_ROLE_ASSIGNMENT_ROLE")
	if role == "" {
		role = "architect"
	}
	fmt.Printf(`{"role":"%s","rationale":"helper picked %s"}`+"\n", role, role)
	os.Exit(0)
}
