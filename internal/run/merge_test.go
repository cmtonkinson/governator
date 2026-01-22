package run

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/testrepos"
	"github.com/cmtonkinson/governator/internal/worktree"
)

func TestExecuteReviewMergeFlow_Success(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root

	originRoot := filepath.Join(repoRoot, "origin.git")
	repo.RunGit(t, "init", "--bare", originRoot)
	repo.RunGit(t, "remote", "add", "origin", originRoot)
	repo.RunGit(t, "push", "-u", "origin", "main")

	task := index.Task{
		ID:    "T-MERGE-001",
		Title: "Add merge flow coverage",
		Role:  "generalist",
		State: index.TaskStateTested,
	}

	repo.RunGit(t, "checkout", "-b", "task-"+task.ID)
	featureFile := filepath.Join(repoRoot, "FEATURE.md")
	if err := os.WriteFile(featureFile, []byte("merge flow\n"), 0o644); err != nil {
		t.Fatalf("write feature file: %v", err)
	}
	repo.RunGit(t, "add", "FEATURE.md")
	repo.RunGit(t, "commit", "-m", "Add feature")

	repo.RunGit(t, "checkout", "main")

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}
	worktreeResult, err := manager.EnsureWorktree(worktree.Spec{
		TaskID:     task.ID,
		Attempt:    1,
		Branch:     "task-" + task.ID,
		BaseBranch: "main",
	})
	if err != nil {
		t.Fatalf("ensure worktree: %v", err)
	}

	input := MergeFlowInput{
		RepoRoot:     repoRoot,
		WorktreePath: worktreeResult.Path,
		Task:         task,
		MainBranch:   "main",
	}

	result, err := ExecuteReviewMergeFlow(input)
	if err != nil {
		t.Fatalf("execute merge flow: %v", err)
	}
	if !result.Success || result.NewState != index.TaskStateDone {
		t.Fatalf("unexpected merge result: %+v", result)
	}

	if branch := strings.TrimSpace(repo.RunGit(t, "rev-parse", "--abbrev-ref", "HEAD")); branch != "main" {
		t.Fatalf("repo root branch = %q, want main", branch)
	}
	if status := strings.TrimSpace(repo.RunGit(t, "status", "--porcelain")); status != "" {
		for _, line := range strings.Split(status, "\n") {
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			path := fields[len(fields)-1]
			if strings.HasPrefix(path, "_governator/") || strings.HasPrefix(path, "origin.git/") {
				continue
			}
			t.Fatalf("repo root dirty after merge: %q", status)
		}
	}
}

func TestExecuteReviewMergeFlow_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		input       MergeFlowInput
		expectError string
	}{
		{
			name:        "empty repo root",
			input:       MergeFlowInput{RepoRoot: "", WorktreePath: "/path", Task: index.Task{ID: "test"}},
			expectError: "repo root is required",
		},
		{
			name:        "empty worktree path",
			input:       MergeFlowInput{RepoRoot: "/path", WorktreePath: "", Task: index.Task{ID: "test"}},
			expectError: "worktree path is required",
		},
		{
			name:        "empty task ID",
			input:       MergeFlowInput{RepoRoot: "/path", WorktreePath: "/path", Task: index.Task{ID: ""}},
			expectError: "task ID is required",
		},
		{
			name:        "empty task title",
			input:       MergeFlowInput{RepoRoot: "/path", WorktreePath: "/path", Task: index.Task{ID: "T-001", Title: ""}},
			expectError: "task title is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ExecuteReviewMergeFlow(tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestExecuteConflictResolutionMergeFlow_InvalidState(t *testing.T) {
	task := index.Task{
		ID:    "test-task-01",
		Title: "Test Task",
		Role:  "generalist",
		State: index.TaskStateTested, // Wrong state - should be resolved
	}

	input := MergeFlowInput{
		RepoRoot:     "/tmp",
		WorktreePath: "/tmp/worktree",
		Task:         task,
		MainBranch:   "main",
	}

	_, err := ExecuteConflictResolutionMergeFlow(input)
	if err == nil {
		t.Fatal("expected error for invalid state, got nil")
	}

	expectedError := "task must be in resolved state"
	if !strings.Contains(err.Error(), expectedError) {
		t.Errorf("expected error containing %q, got %q", expectedError, err.Error())
	}
}

func TestIsRebaseConflict(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "conflict error",
			err:      errors.New("CONFLICT (content): Merge conflict in file.txt"),
			expected: true,
		},
		{
			name:     "could not apply error",
			err:      errors.New("error: could not apply abc123... commit message"),
			expected: true,
		},
		{
			name:     "merge conflict error",
			err:      errors.New("error: merge conflict detected"),
			expected: true,
		},
		{
			name:     "non-conflict error",
			err:      errors.New("fatal: not a git repository"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isRebaseConflict(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestIsMergeConflict(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "conflict error",
			err:      errors.New("CONFLICT (content): Merge conflict in file.txt"),
			expected: true,
		},
		{
			name:     "automatic merge failed error",
			err:      errors.New("Automatic merge failed; fix conflicts and then commit the result"),
			expected: true,
		},
		{
			name:     "merge conflict error",
			err:      errors.New("error: merge conflict detected"),
			expected: true,
		},
		{
			name:     "non-conflict error",
			err:      errors.New("fatal: not a git repository"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isMergeConflict(tt.err)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestRunGitInWorktree_ValidationErrors(t *testing.T) {
	tests := []struct {
		name         string
		worktreePath string
		args         []string
		expectError  string
	}{
		{
			name:         "empty worktree path",
			worktreePath: "",
			args:         []string{"status"},
			expectError:  "worktree path is required",
		},
		{
			name:         "no git arguments",
			worktreePath: "/tmp",
			args:         []string{},
			expectError:  "git arguments are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runGitInWorktree(tt.worktreePath, tt.args...)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}

func TestRunGitInRepo_ValidationErrors(t *testing.T) {
	tests := []struct {
		name        string
		repoRoot    string
		args        []string
		expectError string
	}{
		{
			name:        "empty repo root",
			repoRoot:    "",
			args:        []string{"status"},
			expectError: "repo root is required",
		},
		{
			name:        "no git arguments",
			repoRoot:    "/tmp",
			args:        []string{},
			expectError: "git arguments are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := runGitInRepo(tt.repoRoot, tt.args...)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.expectError) {
				t.Errorf("expected error containing %q, got %q", tt.expectError, err.Error())
			}
		})
	}
}
