// Tests for planning supervisor completion cleanup.
package run

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestRunPlanningSupervisorClearsStateOnCompletion verifies completed supervisors do not persist state.
func TestRunPlanningSupervisorClearsStateOnCompletion(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repoRoot := repo.Root

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}
	if err := SeedPlanningIndex(repoRoot); err != nil {
		t.Fatalf("seed planning index: %v", err)
	}

	indexPath := filepath.Join(repoRoot, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load task index: %v", err)
	}
	updatePlanningTaskState(&idx, "")
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save task index: %v", err)
	}

	if err := RunPlanningSupervisor(repoRoot, PlanningSupervisorOptions{PollInterval: 10 * time.Millisecond}); err != nil {
		t.Fatalf("run planning supervisor: %v", err)
	}

	if _, err := os.Stat(supervisor.PlanningStatePath(repoRoot)); !os.IsNotExist(err) {
		t.Fatalf("planning supervisor state should be cleared, stat err=%v", err)
	}
}

// TestCleanupPlanningBranch verifies the planning branch and worktree are cleaned up.
func TestCleanupPlanningBranch(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repoRoot := repo.Root

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}

	// Create a planning branch and worktree manually to simulate planning state
	if err := runGitInRepo(repoRoot, "checkout", "-b", "planning"); err != nil {
		t.Fatalf("create planning branch: %v", err)
	}
	if err := runGitInRepo(repoRoot, "checkout", "main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}

	worktreePath := filepath.Join(repoRoot, "_governator", "_local-state", "task-planning")
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		t.Fatalf("create worktree parent dir: %v", err)
	}
	if err := runGitInRepo(repoRoot, "worktree", "add", worktreePath, "planning"); err != nil {
		t.Fatalf("create planning worktree: %v", err)
	}

	// Verify planning branch and worktree exist before cleanup
	if err := runGitInRepo(repoRoot, "rev-parse", "--verify", "planning"); err != nil {
		t.Fatalf("planning branch should exist before cleanup: %v", err)
	}
	if _, err := os.Stat(worktreePath); os.IsNotExist(err) {
		t.Fatalf("planning worktree should exist before cleanup")
	}

	// Run cleanup
	if err := cleanupPlanningBranch(repoRoot); err != nil {
		t.Fatalf("cleanup planning branch: %v", err)
	}

	// Verify planning branch and worktree are removed after cleanup
	if err := runGitInRepo(repoRoot, "rev-parse", "--verify", "planning"); err == nil {
		t.Fatalf("planning branch should be deleted after cleanup")
	}
	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("planning worktree should be removed after cleanup")
	}
}

// TestCleanupPlanningBranch_NoWorktree verifies cleanup works when only branch exists.
func TestCleanupPlanningBranch_NoWorktree(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repoRoot := repo.Root

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}

	// Create only the planning branch (no worktree)
	if err := runGitInRepo(repoRoot, "checkout", "-b", "planning"); err != nil {
		t.Fatalf("create planning branch: %v", err)
	}
	if err := runGitInRepo(repoRoot, "checkout", "main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}

	// Run cleanup - should not fail even if worktree doesn't exist
	if err := cleanupPlanningBranch(repoRoot); err != nil {
		t.Fatalf("cleanup planning branch: %v", err)
	}

	// Verify planning branch is removed
	if err := runGitInRepo(repoRoot, "rev-parse", "--verify", "planning"); err == nil {
		t.Fatalf("planning branch should be deleted after cleanup")
	}
}

// TestCleanupPlanningBranch_NoBranch verifies cleanup is idempotent when nothing exists.
func TestCleanupPlanningBranch_NoBranch(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repoRoot := repo.Root

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}

	// Run cleanup when planning branch doesn't exist - should not fail
	if err := cleanupPlanningBranch(repoRoot); err != nil {
		t.Fatalf("cleanup planning branch should not fail when branch doesn't exist: %v", err)
	}
}

// TestRunPlanningSupervisorRegistersExistingTasks verifies plan can complete from existing tasks.
func TestRunPlanningSupervisorRegistersExistingTasks(t *testing.T) {
	t.Parallel()

	t.Run("not_started", func(t *testing.T) {
		t.Parallel()
		repo := testrepos.New(t)
		repoRoot := repo.Root

		if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
			t.Fatalf("init full layout: %v", err)
		}
		writeTestPlanningSpec(t, repoRoot)
		if err := SeedPlanningIndex(repoRoot); err != nil {
			t.Fatalf("seed planning index: %v", err)
		}

		tasksDir := filepath.Join(repoRoot, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0o755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}
		taskPath := filepath.Join(tasksDir, "001-existing-task.md")
		if err := os.WriteFile(taskPath, []byte("# Existing Task\n\nUse existing tasks."), 0o644); err != nil {
			t.Fatalf("write task file: %v", err)
		}

		if err := RunPlanningSupervisor(repoRoot, PlanningSupervisorOptions{
			Stdout:       io.Discard,
			Stderr:       io.Discard,
			PollInterval: 10 * time.Millisecond,
		}); err != nil {
			t.Fatalf("run planning supervisor: %v", err)
		}

		idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
		if err != nil {
			t.Fatalf("load task index: %v", err)
		}
		stateID, err := planningTaskState(idx)
		if err != nil {
			t.Fatalf("planning task state: %v", err)
		}
		if stateID != PlanningCompleteState {
			t.Fatalf("planning state = %q, want %q", stateID, PlanningCompleteState)
		}
		if _, err := findIndexTask(&idx, "001-existing-task"); err != nil {
			t.Fatalf("expected task to be indexed: %v", err)
		}
	})

	t.Run("already_complete", func(t *testing.T) {
		t.Parallel()
		repo := testrepos.New(t)
		repoRoot := repo.Root

		if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
			t.Fatalf("init full layout: %v", err)
		}
		writeTestPlanningSpec(t, repoRoot)
		if err := SeedPlanningIndex(repoRoot); err != nil {
			t.Fatalf("seed planning index: %v", err)
		}

		indexPath := filepath.Join(repoRoot, indexFilePath)
		idx, err := index.Load(indexPath)
		if err != nil {
			t.Fatalf("load task index: %v", err)
		}
		updatePlanningTaskState(&idx, "")
		if err := index.Save(indexPath, idx); err != nil {
			t.Fatalf("save task index: %v", err)
		}

		tasksDir := filepath.Join(repoRoot, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0o755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}
		taskPath := filepath.Join(tasksDir, "002-existing-task.md")
		if err := os.WriteFile(taskPath, []byte("# Existing Task Two\n\nUse existing tasks."), 0o644); err != nil {
			t.Fatalf("write task file: %v", err)
		}

		if err := RunPlanningSupervisor(repoRoot, PlanningSupervisorOptions{
			Stdout:       io.Discard,
			Stderr:       io.Discard,
			PollInterval: 10 * time.Millisecond,
		}); err != nil {
			t.Fatalf("run planning supervisor: %v", err)
		}

		updated, err := index.Load(indexPath)
		if err != nil {
			t.Fatalf("reload task index: %v", err)
		}
		if _, err := findIndexTask(&updated, "002-existing-task"); err != nil {
			t.Fatalf("expected task to be indexed: %v", err)
		}
	})
}
