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
