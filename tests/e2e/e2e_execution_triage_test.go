// Package test provides end-to-end coverage for execution triage behavior.
package test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestE2EExecutionTriage validates backlog triage, DAG application, and execution completion.
func TestE2EExecutionTriage(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root
	t.Logf("Test repo at: %s", repoRoot)
	TrackE2ERepo(t, repoRoot)

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}
	if err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("apply repo migrations: %v", err)
	}

	testWorkerPath, err := filepath.Abs("test-worker.sh")
	if err != nil {
		t.Fatalf("get test worker path: %v", err)
	}

	fixturesPath, err := filepath.Abs("testdata/fixtures/worker-actions.yaml")
	if err != nil {
		t.Fatalf("get fixtures path: %v", err)
	}
	t.Setenv("GOVERNATOR_TEST_FIXTURES", fixturesPath)

	cfg := &config.Config{
		Workers: config.WorkersConfig{
			Commands: config.WorkerCommands{
				Default: []string{testWorkerPath, "{task_path}"},
				Roles:   map[string][]string{},
			},
		},
		Concurrency: config.ConcurrencyConfig{
			Global:      1,
			DefaultRole: 1,
			Roles:       map[string]int{},
		},
		Timeouts: config.TimeoutsConfig{
			WorkerSeconds: 30,
		},
		Retries: config.RetriesConfig{
			MaxAttempts: 1,
		},
		Branches: config.BranchConfig{
			Base: "main",
		},
		ReasoningEffort: config.ReasoningEffortConfig{
			Default: "medium",
			Roles:   map[string]string{},
		},
	}

	configPath := filepath.Join(repoRoot, "_governator/_durable-state/config.json")
	configJSON, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, configJSON, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	governatorPath := filepath.Join(repoRoot, "GOVERNATOR.md")
	if err := os.WriteFile(governatorPath, []byte("Governator execution triage test.\n"), 0o644); err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}

	planningSpec := run.PlanningSpec{
		Version: 2,
		Steps: []run.PlanningStepSpec{
			{
				ID:     "architecture-baseline",
				Name:   "Architecture Baseline",
				Prompt: "_governator/prompts/architecture-baseline.md",
				Role:   "default",
			},
		},
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator/prompts"), 0o755); err != nil {
		t.Fatalf("create prompts dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator/prompts/architecture-baseline.md"), []byte("Test prompt.\n"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	planningJSON, err := json.MarshalIndent(planningSpec, "", "  ")
	if err != nil {
		t.Fatalf("marshal planning spec: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator/planning.json"), planningJSON, 0o644); err != nil {
		t.Fatalf("write planning spec: %v", err)
	}

	tasksDir := filepath.Join(repoRoot, "_governator/tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "task-01.md"), []byte("Implement feature one.\n"), 0o644); err != nil {
		t.Fatalf("write task-01: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "task-02.md"), []byte("Implement feature two.\n"), 0o644); err != nil {
		t.Fatalf("write task-02: %v", err)
	}

	digest, err := digests.Compute(repoRoot)
	if err != nil {
		t.Fatalf("compute digests: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Digests:       digest,
		Tasks: []index.Task{
			{
				ID:    "planning",
				Title: "Planning",
				Path:  "_governator/planning.json",
				Kind:  index.TaskKindPlanning,
				State: run.PlanningCompleteState,
			},
			{
				ID:    "task-01",
				Title: "Task 01",
				Path:  "_governator/tasks/task-01.md",
				Kind:  index.TaskKindExecution,
				State: index.TaskStateBacklog,
				Role:  "default",
				Retries: index.RetryPolicy{
					MaxAttempts: 1,
				},
			},
			{
				ID:    "task-02",
				Title: "Task 02",
				Path:  "_governator/tasks/task-02.md",
				Kind:  index.TaskKindExecution,
				State: index.TaskStateBacklog,
				Role:  "default",
				Retries: index.RetryPolicy{
					MaxAttempts: 1,
				},
			},
		},
	}
	if err := index.Save(filepath.Join(repoRoot, "_governator/_local-state/index.json"), idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	repo.RunGit(t, "add", "GOVERNATOR.md", "_governator")
	repo.RunGit(t, "commit", "-m", "Seed execution triage test data")

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current dir: %v", err)
	}
	defer func() { _ = os.Chdir(originalDir) }()
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir to repo: %v", err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       &stdout,
		Stderr:       &stderr,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("run unified supervisor: %v", err)
	}

	finalIndex, err := index.Load(filepath.Join(repoRoot, "_governator/_local-state/index.json"))
	if err != nil {
		t.Fatalf("load index: %v", err)
	}

	task01 := findTask(t, finalIndex, "task-01")
	task02 := findTask(t, finalIndex, "task-02")

	if task01.State != index.TaskStateMerged || task02.State != index.TaskStateMerged {
		t.Fatalf("expected tasks merged, got %s/%s", task01.State, task02.State)
	}
	if len(task02.Dependencies) != 1 || task02.Dependencies[0] != "task-01" {
		t.Fatalf("expected task-02 dependency on task-01, got %#v", task02.Dependencies)
	}
}

func findTask(t *testing.T, idx index.Index, id string) index.Task {
	t.Helper()
	for _, task := range idx.Tasks {
		if task.ID == id {
			return task
		}
	}
	t.Fatalf("task %s not found", id)
	return index.Task{}
}
