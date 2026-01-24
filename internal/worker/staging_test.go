// Package worker provides tests for worker prompt and environment staging.
package worker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
)

// TestStageEnvAndPromptsHappyPath ensures prompt staging uses stable ordering.
func TestStageEnvAndPromptsHappyPath(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "worker.md"), "role prompt")
	writeFile(t, filepath.Join(root, "_governator", "custom-prompts", "_global.md"), "global prompt")
	writeFile(t, filepath.Join(root, "_governator", "custom-prompts", "worker.md"), "custom prompt")
	writeFile(t, filepath.Join(root, "_governator", "worker-contract.md"), "worker contract")
	writeFile(t, filepath.Join(root, "_governator", "reasoning", "medium.md"), "reasoning prompt")
	taskPath := filepath.Join(root, "_governator", "tasks", "T-001.md")
	writeFile(t, taskPath, "task content")

	task := index.Task{
		ID:   "T-001",
		Path: "_governator/tasks/T-001.md",
		Role: "worker",
	}
	result, err := StageEnvAndPrompts(StageInput{
		RepoRoot:        root,
		WorktreeRoot:    root,
		Task:            task,
		Stage:           roles.StageWork,
		ReasoningEffort: "medium",
	})
	if err != nil {
		t.Fatalf("stage env and prompts: %v", err)
	}

	promptListBytes, err := os.ReadFile(result.PromptListPath)
	if err != nil {
		t.Fatalf("read prompt list: %v", err)
	}
	gotList := strings.Split(strings.TrimSpace(string(promptListBytes)), "\n")
	wantList := []string{
		"_governator/reasoning/medium.md",
		"_governator/worker-contract.md",
		"_governator/roles/worker.md",
		"_governator/custom-prompts/_global.md",
		"_governator/custom-prompts/worker.md",
		"_governator/tasks/T-001.md",
	}
	if len(gotList) != len(wantList) {
		t.Fatalf("prompt list length = %d, want %d", len(gotList), len(wantList))
	}
	for i, want := range wantList {
		if gotList[i] != want {
			t.Fatalf("prompt list[%d] = %q, want %q", i, gotList[i], want)
		}
	}

	promptBytes, err := os.ReadFile(result.PromptPath)
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	prompt := string(promptBytes)
	if !strings.Contains(prompt, "reasoning prompt") {
		t.Fatalf("prompt missing reasoning content: %q", prompt)
	}
	if !strings.Contains(prompt, "role prompt") {
		t.Fatalf("prompt missing role content: %q", prompt)
	}
	if !strings.Contains(prompt, "custom prompt") {
		t.Fatalf("prompt missing custom prompt content: %q", prompt)
	}
	if !strings.Contains(prompt, "task content") {
		t.Fatalf("prompt missing task content: %q", prompt)
	}
	if _, err := os.Stat(result.EnvPath); err != nil {
		t.Fatalf("env file missing: %v", err)
	}
}

// TestStageEnvAndPromptsMissingFile ensures missing prompt files block staging.
func TestStageEnvAndPromptsMissingFile(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "worker.md"), "role prompt")
	writeFile(t, filepath.Join(root, "_governator", "worker-contract.md"), "worker contract")
	writeFile(t, filepath.Join(root, "_governator", "reasoning", "medium.md"), "reasoning prompt")
	task := index.Task{
		ID:   "T-002",
		Path: "_governator/tasks/T-002.md",
		Role: "worker",
	}
	_, err := StageEnvAndPrompts(StageInput{
		RepoRoot:        root,
		WorktreeRoot:    root,
		Task:            task,
		Stage:           roles.StageWork,
		ReasoningEffort: "medium",
	})
	if err == nil {
		t.Fatal("expected error for missing task prompt file")
	}
	if !strings.Contains(err.Error(), "missing prompt file") {
		t.Fatalf("error = %q, want missing prompt file", err.Error())
	}
}

// TestStageEnvAndPromptsFailsWhenReasoningPromptMissing ensures missing reasoning prompts error fast.
func TestStageEnvAndPromptsFailsWhenReasoningPromptMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "worker.md"), "role prompt")
	writeFile(t, filepath.Join(root, "_governator", "worker-contract.md"), "worker contract")
	writeFile(t, filepath.Join(root, "_governator", "tasks", "T-003.md"), "task content")

	task := index.Task{
		ID:   "T-003",
		Path: "_governator/tasks/T-003.md",
		Role: "worker",
	}
	_, err := StageEnvAndPrompts(StageInput{
		RepoRoot:        root,
		WorktreeRoot:    root,
		Task:            task,
		Stage:           roles.StageWork,
		ReasoningEffort: "heavy",
	})
	if err == nil {
		t.Fatal("expected error for missing reasoning prompt file")
	}
	if !strings.Contains(err.Error(), "_governator/reasoning/heavy.md") {
		t.Fatalf("error = %q, want missing reasoning path", err.Error())
	}
}

// TestStageEnvAndPromptsFailsWhenWorkerContractMissing checks the contract prompt is required.
func TestStageEnvAndPromptsFailsWhenWorkerContractMissing(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "worker.md"), "role prompt")
	writeFile(t, filepath.Join(root, "_governator", "reasoning", "medium.md"), "reasoning prompt")
	writeFile(t, filepath.Join(root, "_governator", "tasks", "T-004.md"), "task content")

	task := index.Task{
		ID:   "T-004",
		Path: "_governator/tasks/T-004.md",
		Role: "worker",
	}
	_, err := StageEnvAndPrompts(StageInput{
		RepoRoot:        root,
		WorktreeRoot:    root,
		Task:            task,
		Stage:           roles.StageWork,
		ReasoningEffort: "medium",
	})
	if err == nil {
		t.Fatal("expected error for missing worker contract prompt")
	}
	if !strings.Contains(err.Error(), "_governator/worker-contract.md") {
		t.Fatalf("error = %q, want worker contract path", err.Error())
	}
}

// TestStageEnvAndPromptsValidation ensures input validation works correctly.
func TestStageEnvAndPromptsValidation(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "worker.md"), "role prompt")
	writeFile(t, filepath.Join(root, "_governator", "tasks", "T-001.md"), "task content")

	task := index.Task{
		ID:   "T-001",
		Path: "_governator/tasks/T-001.md",
		Role: "worker",
	}

	tests := []struct {
		name    string
		input   StageInput
		wantErr string
	}{
		{
			name: "empty repo root",
			input: StageInput{
				RepoRoot:     "",
				WorktreeRoot: root,
				Task:         task,
				Stage:        roles.StageWork,
			},
			wantErr: "repo root is required",
		},
		{
			name: "empty worktree root",
			input: StageInput{
				RepoRoot:     root,
				WorktreeRoot: "",
				Task:         task,
				Stage:        roles.StageWork,
			},
			wantErr: "worktree root is required",
		},
		{
			name: "invalid stage",
			input: StageInput{
				RepoRoot:     root,
				WorktreeRoot: root,
				Task:         task,
				Stage:        "invalid",
			},
			wantErr: "unsupported stage",
		},
		{
			name: "empty task id",
			input: StageInput{
				RepoRoot:     root,
				WorktreeRoot: root,
				Task: index.Task{
					ID:   "",
					Path: "_governator/tasks/T-001.md",
					Role: "worker",
				},
				Stage: roles.StageWork,
			},
			wantErr: "task id is required",
		},
		{
			name: "empty task path",
			input: StageInput{
				RepoRoot:     root,
				WorktreeRoot: root,
				Task: index.Task{
					ID:   "T-001",
					Path: "",
					Role: "worker",
				},
				Stage: roles.StageWork,
			},
			wantErr: "task path is required",
		},
		{
			name: "empty role",
			input: StageInput{
				RepoRoot:     root,
				WorktreeRoot: root,
				Task: index.Task{
					ID:   "T-001",
					Path: "_governator/tasks/T-001.md",
					Role: "",
				},
				Stage: roles.StageWork,
			},
			wantErr: "role is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := StageEnvAndPrompts(tt.input)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// TestStageEnvAndPromptsRoleOverride ensures role override works correctly.
func TestStageEnvAndPromptsRoleOverride(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "planner.md"), "planner role prompt")
	writeFile(t, filepath.Join(root, "_governator", "worker-contract.md"), "worker contract")
	writeFile(t, filepath.Join(root, "_governator", "reasoning", "medium.md"), "reasoning prompt")
	writeFile(t, filepath.Join(root, "_governator", "tasks", "T-001.md"), "task content")

	task := index.Task{
		ID:   "T-001",
		Path: "_governator/tasks/T-001.md",
		Role: "worker", // task has worker role
	}
	result, err := StageEnvAndPrompts(StageInput{
		RepoRoot:        root,
		WorktreeRoot:    root,
		Task:            task,
		Stage:           roles.StageWork,
		Role:            "planner", // but we override to planner
		ReasoningEffort: "medium",
	})
	if err != nil {
		t.Fatalf("stage env and prompts: %v", err)
	}

	if result.Env["GOVERNATOR_ROLE"] != "planner" {
		t.Fatalf("env role = %q, want planner", result.Env["GOVERNATOR_ROLE"])
	}
}

// TestStageEnvAndPromptsAllStages ensures all stages generate correct markers.
func TestStageEnvAndPromptsAllStages(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "worker.md"), "role prompt")
	writeFile(t, filepath.Join(root, "_governator", "worker-contract.md"), "worker contract")
	writeFile(t, filepath.Join(root, "_governator", "reasoning", "medium.md"), "reasoning prompt")
	writeFile(t, filepath.Join(root, "_governator", "tasks", "T-001.md"), "task content")

	task := index.Task{
		ID:   "T-001",
		Path: "_governator/tasks/T-001.md",
		Role: "worker",
	}

	stages := []struct {
		stage  roles.Stage
		marker string
	}{
		{roles.StageWork, "worked.md"},
		{roles.StageTest, "tested.md"},
		{roles.StageReview, "reviewed.md"},
		{roles.StageResolve, "resolved.md"},
	}

	for _, s := range stages {
		t.Run(string(s.stage), func(t *testing.T) {
			result, err := StageEnvAndPrompts(StageInput{
				RepoRoot:        root,
				WorktreeRoot:    root,
				Task:            task,
				Stage:           s.stage,
				ReasoningEffort: "medium",
			})
			if err != nil {
				t.Fatalf("stage env and prompts: %v", err)
			}

			promptBytes, err := os.ReadFile(result.PromptPath)
			if err != nil {
				t.Fatalf("read prompt file: %v", err)
			}
			prompt := string(promptBytes)
			if !strings.Contains(prompt, "reasoning prompt") {
				t.Fatalf("prompt missing reasoning content: %q", prompt)
			}
			if !strings.Contains(prompt, "role prompt") {
				t.Fatalf("prompt missing role content: %q", prompt)
			}
		})
	}
}

// writeFile creates the file and parent directories with content.
func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}
