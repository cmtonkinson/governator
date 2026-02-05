// Tests for the end-to-end pipeline that runs bootstrap, planning, and execution.
package run

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/testrepos"
	"github.com/cmtonkinson/governator/internal/worktree"
)

// TestPipelineIntegrationHappyPath covers a full bootstrap, plan, and run execution.
func TestPipelineIntegrationHappyPath(t *testing.T) {
	if os.Getenv("GO_PIPELINE_WORKER_HELPER") == "1" {
		return
	}

	t.Setenv("GO_PIPELINE_WORKER_HELPER", "1")

	workerCommand := []string{os.Args[0], "-test.run=TestPipelineWorkerHelper", "--", "{task_path}"}
	repo := setupPipelineRepo(t, workerCommand)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-PIPE-001", "Pipeline integration task", "worker")
	task := newTestTask("T-PIPE-001", "Pipeline integration task", "worker", taskPath, 10)
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", "_governator/index.json", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add plan outputs")

	indexPath := filepath.Join(repoRoot, "_governator", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	expectedTasks := 1 + len(mergedPlanningTasks(t, repoRoot))
	if len(idx.Tasks) != expectedTasks {
		t.Fatalf("index contains %d tasks, want %d", len(idx.Tasks), expectedTasks)
	}

	if err := prepareWorkedTask(t, repoRoot, &idx, repo, config.Defaults().Branches.Base); err != nil {
		t.Fatalf("prepare worked task: %v", err)
	}

	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save prepared index: %v", err)
	}

	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	result, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	worktreePath, err := manager.WorktreePath("T-PIPE-001")
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}
	waitForExitStatus(t, worktreePath, "T-PIPE-001", roles.StageTest)

	runStdout.Reset()
	runStderr.Reset()
	result, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run collect failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	waitForExitStatus(t, worktreePath, "T-PIPE-001", roles.StageReview)

	runStdout.Reset()
	runStderr.Reset()
	result, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run collect review failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	foundTask, err := findIndexTask(&finalIdx, "T-PIPE-001")
	if err != nil {
		t.Fatalf("find task: %v", err)
	}
	if foundTask.State != index.TaskStateDone {
		t.Fatalf("task state = %q, want %q", foundTask.State, index.TaskStateDone)
	}
	if !strings.Contains(result.Message, "review task(s)") {
		t.Fatalf("expected review stage summary in result message, got %q", result.Message)
	}
}

// TestPipelineIntegrationDrift ensures run halts when planning artifacts drift.
func TestPipelineIntegrationDrift(t *testing.T) {
	if os.Getenv("GO_PIPELINE_WORKER_HELPER") == "1" {
		return
	}

	t.Setenv("GO_PIPELINE_WORKER_HELPER", "1")

	workerCommand := []string{os.Args[0], "-test.run=TestPipelineWorkerHelper", "--", "{task_path}"}
	repo := setupPipelineRepo(t, workerCommand)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-PIPE-001", "Pipeline integration task", "worker")
	task := newTestTask("T-PIPE-001", "Pipeline integration task", "worker", taskPath, 10)
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	governatorPath := filepath.Join(repoRoot, "GOVERNATOR.md")
	if err := os.WriteFile(governatorPath, []byte("# Pipeline fixture\n\nDrift\n"), 0o644); err != nil {
		t.Fatalf("update %s: %v", governatorPath, err)
	}

	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	var err error
	_, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err == nil {
		t.Fatal("expected planning drift error")
	}
	if !errors.Is(err, ErrPlanningDrift) {
		t.Fatalf("error = %v, want ErrPlanningDrift", err)
	}
	output := runStdout.String()
	if !strings.Contains(output, "planning=drift status=blocked") {
		t.Fatalf("stdout = %q, want planning drift prefix", output)
	}
	if !strings.Contains(output, "governator plan") {
		t.Fatalf("stdout = %q, want plan guidance", output)
	}
}

// TestPipelinePlannerHelper emits planner JSON for integration tests.
// TestPipelineWorkerHelper stages marker files for each worker stage.
func TestPipelineWorkerHelper(t *testing.T) {
	if os.Getenv("GO_PIPELINE_WORKER_HELPER") != "1" {
		return
	}
	t.Helper()
	stage := os.Getenv("GOVERNATOR_STAGE")
	marker := markerFileForStage(stage)
	if marker == "" {
		fmt.Fprintf(os.Stderr, "unsupported stage %q\n", stage)
		os.Exit(2)
	}
	stateDir := os.Getenv("GOVERNATOR_WORKER_STATE_PATH")
	if stateDir == "" {
		stateDir = filepath.Join("_governator", "_local-state")
	}
	markerPath := filepath.Join(stateDir, marker)
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := os.WriteFile(markerPath, []byte("pipeline mark\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

// setupPipelineRepo configures a temporary repo with Governator defaults and worker config.
func setupPipelineRepo(t *testing.T, workerCommand []string) *testrepos.TempRepo {
	t.Helper()
	repo := testrepos.New(t)
	if err := config.InitFullLayout(repo.Root, config.InitOptions{}); err != nil {
		t.Fatalf("init layout: %v", err)
	}
	governator := filepath.Join(repo.Root, "GOVERNATOR.md")
	if err := os.WriteFile(governator, []byte("# Governator\n"), 0o644); err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}
	repo.RunGit(t, "add", "GOVERNATOR.md")
	repo.RunGit(t, "commit", "-m", "Add GOVERNATOR")
	rolesDir := filepath.Join(repo.Root, "_governator", "roles")
	if err := os.MkdirAll(rolesDir, 0o755); err != nil {
		t.Fatalf("mkdir roles: %v", err)
	}
	roleFile := filepath.Join(rolesDir, "worker.md")
	if err := os.WriteFile(roleFile, []byte("# Worker role prompt\n"), 0o644); err != nil {
		t.Fatalf("write worker role: %v", err)
	}
	repo.RunGit(t, "add", filepath.Join("_governator", "roles", "worker.md"))
	repo.RunGit(t, "commit", "-m", "Add worker role prompt")
	writePipelineConfig(t, repo.Root, workerCommand)
	repo.RunGit(t, "remote", "add", "origin", repo.Root)
	return repo
}

// writePipelineConfig persists the provided worker command in the repo config.
func writePipelineConfig(t *testing.T, repoRoot string, workerCommand []string) {
	t.Helper()
	cfg := config.Defaults()
	cfg.Workers.Commands.Default = append([]string(nil), workerCommand...)
	cfgPath := filepath.Join(repoRoot, "_governator", "_durable-state", "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(cfgPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// prepareWorkedTask transitions the planned task into a ready state for run execution.
func prepareWorkedTask(t *testing.T, repoRoot string, idx *index.Index, repo *testrepos.TempRepo, baseBranch string) error {
	t.Helper()
	opts := Options{Stdout: io.Discard, Stderr: io.Discard}
	if _, err := EnsureBranchesForOpenTasks(repoRoot, idx, nil, opts, baseBranch); err != nil {
		return fmt.Errorf("ensure branches: %w", err)
	}
	repo.RunGit(t, "checkout", "main")
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return fmt.Errorf("worktree manager: %w", err)
	}
	effectiveBranch := strings.TrimSpace(baseBranch)
	if effectiveBranch == "" {
		effectiveBranch = config.Defaults().Branches.Base
	}
	for i := range idx.Tasks {
		task := &idx.Tasks[i]
		if task.Kind == index.TaskKindPlanning {
			continue
		}
		task.State = index.TaskStateWorked
		task.Attempts.Total = 1
		branchName := fmt.Sprintf("task-%s", task.ID)
		spec := worktree.Spec{
			WorkstreamID: task.ID,
			Branch:       branchName,
			BaseBranch:   effectiveBranch,
		}
		worktreeResult, err := manager.EnsureWorktree(spec)
		if err != nil {
			return fmt.Errorf("ensure worktree: %w", err)
		}
		if err := commitWorktreeChange(t, worktreeResult.Path, task.ID); err != nil {
			return fmt.Errorf("commit worktree change: %w", err)
		}
	}
	return nil
}

// markerFileForStage maps a worker stage to its expected marker file.
func markerFileForStage(stage string) string {
	switch stage {
	case "work":
		return "worked.md"
	case "test":
		return "tested.md"
	case "review":
		return "reviewed.md"
	case "resolve":
		return "resolved.md"
	default:
		return ""
	}
}

func commitWorktreeChange(t *testing.T, worktreePath, taskID string) error {
	t.Helper()
	markerPath := filepath.Join(worktreePath, "_governator", "_local-state", "worked.md")
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		return fmt.Errorf("mkdir marker dir: %w", err)
	}
	if err := os.WriteFile(markerPath, []byte("workstage marker\n"), 0o644); err != nil {
		return fmt.Errorf("write worked marker: %w", err)
	}
	stateMarkerDir := filepath.Join(worktreePath, "_governator", "_local-state", "worker-1-work-worker")
	if err := os.MkdirAll(stateMarkerDir, 0o755); err != nil {
		return fmt.Errorf("mkdir worker state dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stateMarkerDir, "worked.md"), []byte("workstage marker\n"), 0o644); err != nil {
		return fmt.Errorf("write worker state marker: %w", err)
	}
	if _, err := execGitCommand(worktreePath, "add", "_governator/_local-state/worked.md"); err != nil {
		return err
	}
	if _, err := execGitCommand(worktreePath, "commit", "-m", fmt.Sprintf("Work for %s", taskID)); err != nil {
		return err
	}
	return nil
}

func execGitCommand(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

// TestEnsureBranchesForOpenTasks_BatchCreation verifies batch branch creation logic.
// This test ensures that:
// - Multiple task branches are created efficiently
// - The repository remains on the main branch after creation
// - All branches point to the same base commit
func TestEnsureBranchesForOpenTasks_BatchCreation(t *testing.T) {
	// Setup test repository
	repo := testrepos.New(t)
	repoRoot := repo.Root

	// Initialize governator structure
	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init layout: %v", err)
	}

	// Create multiple triaged tasks
	tasks := []index.Task{
		{
			ID:    "test-001",
			Role:  "default",
			Title: "First test task",
			Kind:  index.TaskKindExecution,
			State: index.TaskStateTriaged,
		},
		{
			ID:    "test-002",
			Role:  "default",
			Title: "Second test task",
			Kind:  index.TaskKindExecution,
			State: index.TaskStateTriaged,
		},
		{
			ID:    "test-003",
			Role:  "default",
			Title: "Third test task",
			Kind:  index.TaskKindExecution,
			State: index.TaskStateTriaged,
		},
	}

	// Create index with tasks
	idx := index.Index{
		Tasks: tasks,
	}

	// Save index
	indexPath := filepath.Join(repoRoot, "_governator", "index.json")
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	// Ensure branches
	opts := Options{
		Stdout: io.Discard,
		Stderr: io.Discard,
	}
	result, err := EnsureBranchesForOpenTasks(repoRoot, &idx, nil, opts, "main")
	if err != nil {
		t.Fatalf("EnsureBranchesForOpenTasks failed: %v", err)
	}

	// Verify result counts
	if result.BranchesCreated != 3 {
		t.Errorf("Expected 3 branches created, got %d", result.BranchesCreated)
	}
	if result.BranchesSkipped != 0 {
		t.Errorf("Expected 0 branches skipped, got %d", result.BranchesSkipped)
	}

	// Verify all branches exist
	for _, task := range tasks {
		branchName := TaskBranchName(task)
		output, err := execGitCommand(repoRoot, "show-ref", "--verify", "--quiet", "refs/heads/"+branchName)
		if err != nil {
			t.Errorf("Branch %s should exist but got error: %v (output: %s)", branchName, err, output)
		}
	}

	// Verify we're on main branch
	currentBranch, err := execGitCommand(repoRoot, "branch", "--show-current")
	if err != nil {
		t.Fatalf("Failed to get current branch: %v", err)
	}
	currentBranch = strings.TrimSpace(currentBranch)
	if currentBranch != "main" {
		t.Errorf("Expected to be on main branch, but on %s", currentBranch)
	}

	// Verify all task branches point to main (same commit)
	mainCommit, err := execGitCommand(repoRoot, "rev-parse", "main")
	if err != nil {
		t.Fatalf("Failed to get main commit: %v", err)
	}
	mainCommit = strings.TrimSpace(mainCommit)

	for _, task := range tasks {
		branchName := TaskBranchName(task)
		branchCommit, err := execGitCommand(repoRoot, "rev-parse", branchName)
		if err != nil {
			t.Fatalf("Failed to get commit for branch %s: %v", branchName, err)
		}
		branchCommit = strings.TrimSpace(branchCommit)
		if branchCommit != mainCommit {
			t.Errorf("Branch %s commit %s doesn't match main commit %s", branchName, branchCommit, mainCommit)
		}
	}

	// Test idempotency - calling again should skip all branches
	result2, err := EnsureBranchesForOpenTasks(repoRoot, &idx, nil, opts, "main")
	if err != nil {
		t.Fatalf("Second EnsureBranchesForOpenTasks failed: %v", err)
	}

	if result2.BranchesCreated != 0 {
		t.Errorf("Expected 0 branches created on second run, got %d", result2.BranchesCreated)
	}
	if result2.BranchesSkipped != 3 {
		t.Errorf("Expected 3 branches skipped on second run, got %d", result2.BranchesSkipped)
	}

	// Verify still on main branch
	currentBranch2, err := execGitCommand(repoRoot, "branch", "--show-current")
	if err != nil {
		t.Fatalf("Failed to get current branch after second run: %v", err)
	}
	currentBranch2 = strings.TrimSpace(currentBranch2)
	if currentBranch2 != "main" {
		t.Errorf("Expected to be on main branch after second run, but on %s", currentBranch2)
	}
}
