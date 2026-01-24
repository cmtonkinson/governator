package run

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/index"
)

// setupBranchTestRepo creates a test git repository with initial commit for branch tests
func setupBranchTestRepo(t *testing.T) string {
	t.Helper()

	tempDir := t.TempDir()
	repoRoot := filepath.Join(tempDir, "test-repo")

	// Initialize git repository
	if err := os.MkdirAll(repoRoot, 0755); err != nil {
		t.Fatalf("Failed to create test repo directory: %v", err)
	}

	// Initialize git repo
	runGitCmd(t, repoRoot, "init")
	runGitCmd(t, repoRoot, "config", "user.name", "Test User")
	runGitCmd(t, repoRoot, "config", "user.email", "test@example.com")

	// Create initial commit
	readmeFile := filepath.Join(repoRoot, "README.md")
	if err := os.WriteFile(readmeFile, []byte("# Test Repo\n"), 0644); err != nil {
		t.Fatalf("Failed to create README: %v", err)
	}
	runGitCmd(t, repoRoot, "add", "README.md")
	runGitCmd(t, repoRoot, "commit", "-m", "Initial commit")

	// Ensure we're on main branch (rename if needed)
	runGitCmd(t, repoRoot, "branch", "-M", "main")

	return repoRoot
}

func TestBranchLifecycleManager_CreateTaskBranch(t *testing.T) {
	repoRoot := setupBranchTestRepo(t)

	// Create audit logger
	auditor, err := audit.NewLogger(repoRoot, os.Stderr)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	// Create branch lifecycle manager
	blm := NewBranchLifecycleManager(repoRoot, auditor)

	// Test task
	task := index.Task{
		ID:    "test-task-01",
		Role:  "default",
		Title: "Test Task One",
	}

	t.Run("CreateTaskBranch_Success", func(t *testing.T) {
		err := blm.CreateTaskBranch(task, "main")
		if err != nil {
			t.Fatalf("CreateTaskBranch failed: %v", err)
		}

		// Verify branch was created
		branchName := TaskBranchName(task)
		exists, err := blm.BranchExists(branchName)
		if err != nil {
			t.Fatalf("Failed to check if branch exists: %v", err)
		}
		if !exists {
			t.Errorf("Expected branch %s to exist", branchName)
		}
	})

	t.Run("CreateTaskBranch_AlreadyExists", func(t *testing.T) {
		// Try to create the same branch again
		err := blm.CreateTaskBranch(task, "main")
		if err != nil {
			t.Fatalf("CreateTaskBranch should not fail when branch already exists: %v", err)
		}
	})

	t.Run("CreateTaskBranch_EmptyTaskID", func(t *testing.T) {
		emptyTask := index.Task{ID: "", Role: "default"}
		err := blm.CreateTaskBranch(emptyTask, "main")
		if err == nil {
			t.Error("Expected error for empty task ID")
		}
	})
}

func TestBranchLifecycleManager_CleanupTaskBranch(t *testing.T) {
	repoRoot := setupBranchTestRepo(t)

	// Create audit logger
	auditor, err := audit.NewLogger(repoRoot, os.Stderr)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	// Create branch lifecycle manager
	blm := NewBranchLifecycleManager(repoRoot, auditor)

	// Test task
	task := index.Task{
		ID:    "test-task-02",
		Role:  "default",
		Title: "Cleanup Task",
	}

	t.Run("CleanupTaskBranch_Success", func(t *testing.T) {
		// First create a branch
		err := blm.CreateTaskBranch(task, "main")
		if err != nil {
			t.Fatalf("Failed to create task branch: %v", err)
		}

		// Verify branch exists
		branchName := TaskBranchName(task)
		exists, err := blm.BranchExists(branchName)
		if err != nil {
			t.Fatalf("Failed to check if branch exists: %v", err)
		}
		if !exists {
			t.Fatalf("Expected branch %s to exist before cleanup", branchName)
		}

		// Clean up the branch
		err = blm.CleanupTaskBranch(task)
		if err != nil {
			t.Fatalf("CleanupTaskBranch failed: %v", err)
		}

		// Verify branch was deleted
		exists, err = blm.BranchExists(branchName)
		if err != nil {
			t.Fatalf("Failed to check if branch exists after cleanup: %v", err)
		}
		if exists {
			t.Errorf("Expected branch %s to be deleted", branchName)
		}
	})

	t.Run("CleanupTaskBranch_NonExistentBranch", func(t *testing.T) {
		nonExistentTask := index.Task{
			ID:   "non-existent-task",
			Role: "default",
		}

		// Try to clean up a branch that doesn't exist
		err := blm.CleanupTaskBranch(nonExistentTask)
		if err != nil {
			t.Fatalf("CleanupTaskBranch should not fail for non-existent branch: %v", err)
		}
	})
}

func TestBranchLifecycleManager_EnsureTaskBranch(t *testing.T) {
	repoRoot := setupBranchTestRepo(t)

	// Create audit logger
	auditor, err := audit.NewLogger(repoRoot, os.Stderr)
	if err != nil {
		t.Fatalf("Failed to create audit logger: %v", err)
	}

	// Create branch lifecycle manager
	blm := NewBranchLifecycleManager(repoRoot, auditor)

	// Test task
	task := index.Task{
		ID:    "test-task-03",
		Role:  "default",
		Title: "Ensure Task Branch",
	}

	t.Run("EnsureTaskBranch_CreateNew", func(t *testing.T) {
		err := blm.EnsureTaskBranch(task, "main")
		if err != nil {
			t.Fatalf("EnsureTaskBranch failed: %v", err)
		}

		// Verify branch was created
		branchName := TaskBranchName(task)
		exists, err := blm.BranchExists(branchName)
		if err != nil {
			t.Fatalf("Failed to check if branch exists: %v", err)
		}
		if !exists {
			t.Errorf("Expected branch %s to exist", branchName)
		}
	})

	t.Run("EnsureTaskBranch_ExistingBranch", func(t *testing.T) {
		// Try to ensure the same branch again
		err := blm.EnsureTaskBranch(task, "main")
		if err != nil {
			t.Fatalf("EnsureTaskBranch should not fail for existing branch: %v", err)
		}
	})
}

func TestBranchLifecycleManager_GetTaskBranchName(t *testing.T) {
	blm := &BranchLifecycleManager{}

	tests := []struct {
		name     string
		task     index.Task
		expected string
	}{
		{
			name: "noTitle",
			task: index.Task{
				ID:    "task-01",
				Role:  "default",
				Title: "",
			},
			expected: "task-task-01",
		},
		{
			name: "withTitle",
			task: index.Task{
				ID:    "complex-task",
				Role:  "default",
				Title: "Add logging steps",
			},
			expected: "task-complex-task-add-logging-steps",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := blm.GetTaskBranchName(test.task)
			if result != test.expected {
				t.Errorf("Expected %s, got %s", test.expected, result)
			}
		})
	}
}

func TestBranchLifecycleManager_BranchExists(t *testing.T) {
	repoRoot := setupBranchTestRepo(t)

	// Create branch lifecycle manager
	blm := NewBranchLifecycleManager(repoRoot, nil)

	t.Run("BranchExists_ExistingBranch", func(t *testing.T) {
		exists, err := blm.BranchExists("main")
		if err != nil {
			t.Fatalf("BranchExists failed: %v", err)
		}
		if !exists {
			t.Error("Expected main branch to exist")
		}
	})

	t.Run("BranchExists_NonExistentBranch", func(t *testing.T) {
		exists, err := blm.BranchExists("non-existent-branch")
		if err != nil {
			t.Fatalf("BranchExists failed: %v", err)
		}
		if exists {
			t.Error("Expected non-existent-branch to not exist")
		}
	})

	t.Run("BranchExists_EmptyBranchName", func(t *testing.T) {
		_, err := blm.BranchExists("")
		if err == nil {
			t.Error("Expected error for empty branch name")
		}
	})
}

// Helper function to run git commands in tests
func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, string(output))
	}
}
