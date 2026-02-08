// Package run contains tests for planning controller completion behavior.
package run

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/phase"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

func TestPlanningControllerAdvanceFinalStepAllowsIdempotentInventory(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	writeTestPlanningSpec(t, repo.Root)

	taskPath := writeInventoryTaskMarkdown(t, repo.Root, "001-existing.md", "# Existing Task\n\nAlready indexed.")
	existingTask := index.Task{
		ID:       "001-existing",
		Title:    "Existing Task",
		Path:     taskPath,
		Kind:     index.TaskKindExecution,
		State:    index.TaskStateBacklog,
		Role:     index.Role("default"),
		Retries:  index.RetryPolicy{MaxAttempts: 3},
		Attempts: index.AttemptCounters{},
		Order:    1,
	}
	idx := seedPlanningControllerIndex(t, repo.Root, []index.Task{existingTask})

	controller, finalStep := newPlanningControllerFixture(t, repo.Root, idx)
	advanced, err := controller.Advance(finalStep, workstreamCollectResult{Completed: true})
	if err != nil {
		t.Fatalf("Advance returned unexpected error: %v", err)
	}
	if !advanced {
		t.Fatal("expected planning controller to advance on final step")
	}
	if len(controller.idx.Tasks) != 2 {
		t.Fatalf("len(controller.idx.Tasks) = %d, want 2 (planning + existing execution task)", len(controller.idx.Tasks))
	}
	stateID, err := planningTaskState(*controller.idx)
	if err != nil {
		t.Fatalf("planningTaskState failed: %v", err)
	}
	if stateID != PlanningCompleteState {
		t.Fatalf("planning state = %q, want %q", stateID, PlanningCompleteState)
	}
}

func TestPlanningControllerAdvanceFinalStepRequiresExecutionTasksAfterInventory(t *testing.T) {
	t.Parallel()

	if os.Getuid() == 0 {
		t.Skip("skipping permission test when running as root")
	}

	repo := testrepos.New(t)
	writeTestPlanningSpec(t, repo.Root)

	unreadable := filepath.Join(repo.Root, "_governator", "tasks", "001-unreadable.md")
	if err := os.MkdirAll(filepath.Dir(unreadable), 0o755); err != nil {
		t.Fatalf("mkdir tasks dir: %v", err)
	}
	if err := os.WriteFile(unreadable, []byte("# Unreadable\n\nContent"), 0o644); err != nil {
		t.Fatalf("write unreadable task file: %v", err)
	}
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatalf("chmod unreadable task file: %v", err)
	}
	defer os.Chmod(unreadable, 0o644)

	idx := seedPlanningControllerIndex(t, repo.Root, nil)
	controller, finalStep := newPlanningControllerFixture(t, repo.Root, idx)

	advanced, err := controller.Advance(finalStep, workstreamCollectResult{Completed: true})
	if err == nil {
		t.Fatal("expected Advance to fail when no execution tasks exist after inventory")
	}
	if advanced {
		t.Fatal("expected advanced=false on inventory-without-execution-tasks failure")
	}
	if !strings.Contains(err.Error(), "planning completion requires at least one execution task in the task index after inventory") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func newPlanningControllerFixture(t *testing.T, repoRoot string, idx *index.Index) (*planningController, workstreamStep) {
	t.Helper()

	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("load planning task: %v", err)
	}
	finalStep, ok := planning.stepForPhase(phase.PhaseTaskPlanning)
	if !ok {
		t.Fatal("missing task-planning step in planning spec")
	}

	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("new in-flight store: %v", err)
	}
	runner := newPhaseRunner(repoRoot, config.Defaults(), Options{Stdout: io.Discard, Stderr: io.Discard}, inFlightStore, inflight.Set{}, planning)
	return newPlanningController(runner, idx), finalStep
}

func seedPlanningControllerIndex(t *testing.T, repoRoot string, executionTasks []index.Task) *index.Index {
	t.Helper()

	digestSet, err := digests.Compute(repoRoot)
	if err != nil {
		t.Fatalf("compute digests: %v", err)
	}
	planTask := index.Task{
		ID:       planningIndexTaskID,
		Title:    "Planning",
		Path:     planningSpecFilePath,
		Kind:     index.TaskKindPlanning,
		State:    index.TaskState("task-planning"),
		Retries:  index.RetryPolicy{MaxAttempts: 1},
		Attempts: index.AttemptCounters{},
	}
	tasks := append([]index.Task{planTask}, executionTasks...)
	seed := index.Index{
		SchemaVersion: taskIndexSchemaVersion,
		Digests:       digestSet,
		Tasks:         tasks,
	}
	indexPath := filepath.Join(repoRoot, indexFilePath)
	if err := index.Save(indexPath, seed); err != nil {
		t.Fatalf("seed index: %v", err)
	}
	loaded, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload seeded index: %v", err)
	}
	return &loaded
}

func writeInventoryTaskMarkdown(t *testing.T, repoRoot string, name string, content string) string {
	t.Helper()

	taskPath := filepath.Join(repoRoot, "_governator", "tasks", name)
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o755); err != nil {
		t.Fatalf("mkdir task dir: %v", err)
	}
	if err := os.WriteFile(taskPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write task markdown: %v", err)
	}
	return filepath.ToSlash(filepath.Join("_governator", "tasks", name))
}
