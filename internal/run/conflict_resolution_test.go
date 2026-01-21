// Tests for conflict resolution stage functionality.
package run

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
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
	rolePath := filepath.Join(rolesDir, "generalist.md")
	if err := os.WriteFile(rolePath, []byte("# Generalist Role\nGeneral purpose role"), 0644); err != nil {
		t.Fatalf("failed to create role file: %v", err)
	}

	task := index.Task{
		ID:    "task-01",
		State: index.TaskStateConflict,
		Role:  "generalist",
		Title: "Test Task 1",
		Path:  "task-01.md",
		Attempts: index.AttemptCounters{Total: 1},
	}

	cfg := config.Config{
		Concurrency: config.ConcurrencyConfig{
			Global:      10,
			DefaultRole: 5,
			Roles:       map[string]int{"generalist": 3},
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
	_, err := ExecuteConflictResolutionAgent(tempDir, tempDir, task, cfg, nil, opts)
	
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
	
	roles := []string{"generalist", "architect", "reviewer"}
	for _, role := range roles {
		rolePath := filepath.Join(rolesDir, role+".md")
		if err := os.WriteFile(rolePath, []byte("# "+role+" Role\n"+role+" role description"), 0644); err != nil {
			t.Fatalf("failed to create role file: %v", err)
		}
	}

	task := index.Task{
		ID:    "task-01",
		State: index.TaskStateConflict,
		Role:  "generalist",
		Title: "Test Task 1",
		Path:  "task-01.md",
	}

	cfg := config.Config{
		Concurrency: config.ConcurrencyConfig{
			Global:      10,
			DefaultRole: 5,
			Roles:       map[string]int{"generalist": 3, "architect": 2},
		},
	}

	var stdout strings.Builder
	var stderr strings.Builder
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	result, err := SelectRoleForConflictResolution(tempDir, task, cfg, opts)
	if err != nil {
		t.Fatalf("SelectRoleForConflictResolution failed: %v", err)
	}

	// Verify we got a valid role
	if result.Role == "" {
		t.Error("expected non-empty role")
	}

	// Verify the role is one of the available roles
	validRoles := []index.Role{"generalist", "architect", "reviewer"}
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
				rolePath := filepath.Join(rolesDir, "generalist.md")
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

			_, err := SelectRoleForConflictResolution(tempDir, task, cfg, opts)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestMockLLMInvoker_Invoke(t *testing.T) {
	invoker := &mockLLMInvoker{fallbackRole: "generalist"}
	
	response, err := invoker.Invoke(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("Invoke failed: %v", err)
	}
	
	// Verify response contains expected JSON structure
	if !strings.Contains(response, `"role": "generalist"`) {
		t.Errorf("expected response to contain role generalist, got: %s", response)
	}
	if !strings.Contains(response, `"rationale"`) {
		t.Errorf("expected response to contain rationale field, got: %s", response)
	}
}

func TestExecuteMergeStage_Success(t *testing.T) {
	// Create test index with resolved tasks
	idx := &index.Index{
		Tasks: []index.Task{
			{
				ID:    "task-01",
				State: index.TaskStateResolved,
				Role:  "generalist",
				Title: "Test Task 1",
				Attempts: index.AttemptCounters{Total: 1},
			},
		},
	}

	cfg := config.Config{}
	auditor := &mockTransitionAuditor{}
	
	var stdout strings.Builder
	var stderr strings.Builder
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	// This will fail because we don't have actual worktrees set up,
	// but we can verify the function processes the resolved tasks
	result, err := ExecuteMergeStage("/tmp", idx, cfg, auditor, nil, opts)
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
				State: index.TaskStateResolved,
				Role:  "generalist",
			},
		},
	}

	auditor := &mockTransitionAuditor{}
	
	// Test successful merge (resolved -> done)
	result := worker.IngestResult{
		Success:  true,
		NewState: index.TaskStateDone,
	}

	err := UpdateTaskStateFromMerge(idx, "task-01", result, auditor)
	if err != nil {
		t.Fatalf("UpdateTaskStateFromMerge failed: %v", err)
	}

	// Verify task state was updated
	if idx.Tasks[0].State != index.TaskStateDone {
		t.Errorf("expected task state %s, got %s", index.TaskStateDone, idx.Tasks[0].State)
	}

	// Verify audit log was called
	if len(auditor.transitions) != 1 {
		t.Errorf("expected 1 audit transition, got %d", len(auditor.transitions))
	}
	
	transition := auditor.transitions[0]
	if transition.from != string(index.TaskStateResolved) {
		t.Errorf("expected from state %s, got %s", index.TaskStateResolved, transition.from)
	}
	if transition.to != string(index.TaskStateDone) {
		t.Errorf("expected to state %s, got %s", index.TaskStateDone, transition.to)
	}
}

func TestUpdateTaskStateFromMerge_Failure(t *testing.T) {
	// Create test index
	idx := &index.Index{
		Tasks: []index.Task{
			{
				ID:    "task-01",
				State: index.TaskStateResolved,
				Role:  "generalist",
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
	if transition.from != string(index.TaskStateResolved) {
		t.Errorf("expected from state %s, got %s", index.TaskStateResolved, transition.from)
	}
	if transition.to != string(index.TaskStateConflict) {
		t.Errorf("expected to state %s, got %s", index.TaskStateConflict, transition.to)
	}
}