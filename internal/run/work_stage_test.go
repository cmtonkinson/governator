// Tests for work stage orchestration.
package run

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/scheduler"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestExecuteWorkStageHappyPath ensures open tasks are processed successfully in the work stage.
func TestExecuteWorkStageHappyPath(t *testing.T) {
	if os.Getenv("GO_WORK_STAGE_HELPER") == "1" {
		return
	}

	t.Setenv("GO_WORK_STAGE_HELPER", "1")

	repo := testrepos.New(t)
	repoRoot := repo.Root

	roleDir := filepath.Join(repoRoot, "_governator", "roles")
	if err := os.MkdirAll(roleDir, 0o755); err != nil {
		t.Fatalf("create roles dir: %v", err)
	}
	rolePath := filepath.Join(roleDir, "worker.md")
	if err := os.WriteFile(rolePath, []byte("# Worker\n"), 0o644); err != nil {
		t.Fatalf("write role prompt: %v", err)
	}

	taskDir := filepath.Join(repoRoot, "_governator", "tasks")
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}
	taskPath := filepath.Join(taskDir, "T-001-work.md")
	if err := os.WriteFile(taskPath, []byte("# Task\n"), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	repo.RunGit(t, "add", "_governator/roles/worker.md", "_governator/tasks/T-001-work.md")
	repo.RunGit(t, "commit", "-m", "Add role and task prompts")

	workerCommand := []string{os.Args[0], "-test.run=TestWorkStageWorkerHelper", "--", "{task_path}"}
	cfg := config.Defaults()
	cfg.Workers.Commands.Default = workerCommand
	cfg.Concurrency.Global = 1
	cfg.Concurrency.DefaultRole = 1

	idx := index.Index{
		Tasks: []index.Task{
			{
				ID:    "T-001",
				Path:  "_governator/tasks/T-001-work.md",
				Kind:  index.TaskKindExecution,
				State: index.TaskStateOpen,
				Role:  "worker",
			},
		},
	}

	caps := scheduler.RoleCapsFromConfig(cfg)
	var stdout, stderr bytes.Buffer
	opts := Options{Stdout: &stdout, Stderr: &stderr}
	inFlight := inflight.Set{}
	result, err := ExecuteWorkStage(repoRoot, &idx, cfg, caps, inFlight, nil, nil, nil, opts)
	if err != nil {
		t.Fatalf("execute work stage: %v", err)
	}
	if result.TasksDispatched != 1 {
		t.Fatalf("tasks dispatched = %d, want 1", result.TasksDispatched)
	}
	if idx.Tasks[0].State != index.TaskStateOpen {
		t.Fatalf("task state = %q, want %q", idx.Tasks[0].State, index.TaskStateOpen)
	}
	if idx.Tasks[0].Attempts.Total != 1 {
		t.Fatalf("attempts total = %d, want 1", idx.Tasks[0].Attempts.Total)
	}
	if !inFlight.Contains("T-001") {
		t.Fatalf("expected task to be in-flight after dispatch")
	}

	worktreePath := result.WorktreePaths["T-001"]
	waitForExitStatus(t, worktreePath, "T-001", roles.StageWork)

	result, err = ExecuteWorkStage(repoRoot, &idx, cfg, caps, inFlight, nil, nil, nil, opts)
	if err != nil {
		t.Fatalf("execute work stage collect: %v", err)
	}
	if result.TasksWorked != 1 {
		t.Fatalf("tasks worked = %d, want 1", result.TasksWorked)
	}
	if idx.Tasks[0].State != index.TaskStateWorked {
		t.Fatalf("task state = %q, want %q", idx.Tasks[0].State, index.TaskStateWorked)
	}
	if inFlight.Contains("T-001") {
		t.Fatalf("expected task to be removed from in-flight after completion")
	}
}

// TestWorkStageWorkerHelper writes the worked marker and creates a commit for the work stage.
func TestWorkStageWorkerHelper(t *testing.T) {
	if os.Getenv("GO_WORK_STAGE_HELPER") != "1" {
		return
	}
	t.Helper()

	markerBase := os.Getenv("GOVERNATOR_WORKER_STATE_PATH")
	if markerBase == "" {
		markerBase = filepath.Join("_governator", "_local-state")
	}
	markerPath := filepath.Join(markerBase, "worked.md")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		t.Fatalf("create marker dir: %v", err)
	}
	if err := os.WriteFile(markerPath, []byte("work complete\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if err := runGitCommandWithOutput("add", markerPath); err != nil {
		t.Fatalf("git add: %v", err)
	}
	if err := runGitCommandWithOutput("commit", "-m", "Work stage complete"); err != nil {
		t.Fatalf("git commit: %v", err)
	}
	os.Exit(0)
}

// runGitCommandWithOutput runs git in the current directory and returns an error with stderr.
func runGitCommandWithOutput(args ...string) error {
	cmd := exec.Command("git", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}
