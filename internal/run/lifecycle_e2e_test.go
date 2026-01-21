// Tests for lifecycle-oriented end-to-end flows covering bootstrapped planning, run execution,
// and role-driven stage transitions.
package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/plan"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

const lifecyclePlannerOutput = `{
  "schema_version": 1,
  "kind": "planner_output",
  "architecture_baseline": {
    "schema_version": 1,
    "kind": "architecture_baseline",
    "mode": "synthesis",
    "summary": "Lifecycle integration plan",
    "sources": [
      "GOVERNATOR.md"
    ]
  },
  "roadmap": {
    "schema_version": 1,
    "kind": "roadmap_decomposition",
    "depth_policy": "epic->task",
    "width_policy": "1 day",
    "items": [
      {
        "id": "epic-002",
        "title": "Lifecycle coverage",
        "type": "epic",
        "order": 20
      }
    ]
  },
  "tasks": {
    "schema_version": 1,
    "kind": "task_generation",
    "tasks": [
      {
        "id": "T-LIFE-001",
        "title": "Lifecycle task",
        "summary": "Walk the sequence from worked through reviewed to done.",
        "role": "worker",
        "dependencies": [],
        "order": 10,
        "overlap": [],
        "acceptance_criteria": [
          "Lifecycle states progress in order",
          "Single worker role handles implementation"
        ],
        "tests": [
          "Lifecycle integration scenario"
        ]
      }
    ]
  }
}`

const lifecycleTaskCount = 1

func TestLifecycleEndToEndHappyPath(t *testing.T) {
	if os.Getenv("GO_LIFECYCLE_PLANNER_HELPER") == "1" || os.Getenv("GO_LIFECYCLE_WORKER_HELPER") == "1" {
		return
	}

	t.Setenv("GO_LIFECYCLE_PLANNER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "success")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 2)
	repoRoot := repo.Root

	plannerCommand := []string{os.Args[0], "-test.run=TestLifecyclePlannerHelper", "--", "{task_path}"}
	var planStdout bytes.Buffer
	var planStderr bytes.Buffer
	planResult, err := plan.Run(repoRoot, plan.Options{
		PlannerCommand: plannerCommand,
		Stdout:         &planStdout,
		Stderr:         &planStderr,
	})
	if err != nil {
		t.Fatalf("plan.Run failed: %v, stdout=%q, stderr=%q", err, planStdout.String(), planStderr.String())
	}
	if planResult.TaskCount != lifecycleTaskCount {
		t.Fatalf("plan returned %d tasks, want %d", planResult.TaskCount, lifecycleTaskCount)
	}

	repo.RunGit(t, "add", "_governator/task-index.json", "_governator/plan", "_governator/tasks")
	repo.RunGit(t, "commit", "-m", "Add lifecycle plan outputs")

	indexPath := filepath.Join(repoRoot, "_governator", "task-index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(idx.Tasks) != lifecycleTaskCount {
		t.Fatalf("index contains %d tasks, want %d", len(idx.Tasks), lifecycleTaskCount)
	}

	if err := prepareWorkedTask(t, repoRoot, &idx, repo); err != nil {
		t.Fatalf("prepare worked tasks: %v", err)
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
	if !strings.Contains(result.Message, "review task(s)") {
		t.Fatalf("result message = %q, want review stage summary", result.Message)
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	for _, task := range finalIdx.Tasks {
		if task.State != index.TaskStateDone {
			t.Fatalf("task %q role=%q state = %q, want %q; stdout=%q stderr=%q",
				task.ID, task.Role, task.State, index.TaskStateDone, runStdout.String(), runStderr.String())
		}
		markerLine := fmt.Sprintf("task_id=%s role=%s event=task.transition from=worked to=tested", task.ID, task.Role)
		assertAuditContains(t, repoRoot, markerLine)
		doneLine := fmt.Sprintf("task_id=%s role=%s event=task.transition from=tested to=done", task.ID, task.Role)
		assertAuditContains(t, repoRoot, doneLine)
	}

	assertAuditContains(t, repoRoot, "event=task.transition from=worked to=tested")
	assertAuditContains(t, repoRoot, "event=task.transition from=tested to=done")
}

func TestLifecycleEndToEndTimeoutResume(t *testing.T) {
	if os.Getenv("GO_LIFECYCLE_PLANNER_HELPER") == "1" || os.Getenv("GO_LIFECYCLE_WORKER_HELPER") == "1" {
		return
	}

	t.Setenv("GO_LIFECYCLE_PLANNER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "timeout")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 1)
	repoRoot := repo.Root

	plannerCommand := []string{os.Args[0], "-test.run=TestLifecyclePlannerHelper", "--", "{task_path}"}
	if _, err := plan.Run(repoRoot, plan.Options{PlannerCommand: plannerCommand}); err != nil {
		t.Fatalf("plan.Run failed: %v", err)
	}

	repo.RunGit(t, "add", "_governator/task-index.json", "_governator/plan", "_governator/tasks")
	repo.RunGit(t, "commit", "-m", "Add lifecycle plan outputs")

	indexPath := filepath.Join(repoRoot, "_governator", "task-index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if len(idx.Tasks) != lifecycleTaskCount {
		t.Fatalf("index contains %d tasks, want %d", len(idx.Tasks), lifecycleTaskCount)
	}

	if err := prepareWorkedTask(t, repoRoot, &idx, repo); err != nil {
		t.Fatalf("prepare worked tasks: %v", err)
	}
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save prepared index: %v", err)
	}

	var timeoutStdout bytes.Buffer
	var timeoutStderr bytes.Buffer
	if _, err := Run(repoRoot, Options{Stdout: &timeoutStdout, Stderr: &timeoutStderr}); err != nil {
		t.Fatalf("first run (timeout) failed: %v", err)
	}

	timeoutIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index after timeout: %v", err)
	}
	for _, task := range timeoutIdx.Tasks {
		if task.State != index.TaskStateBlocked {
			t.Fatalf("task %q state after timeout = %q, want %q", task.ID, task.State, index.TaskStateBlocked)
		}
	}

	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "success")
	var resumeStdout bytes.Buffer
	var resumeStderr bytes.Buffer
	resumeResult, err := Run(repoRoot, Options{Stdout: &resumeStdout, Stderr: &resumeStderr})
	if err != nil {
		t.Fatalf("second run (resume) failed: %v, stdout=%q, stderr=%q", err, resumeStdout.String(), resumeStderr.String())
	}
	if resumeResult.ResumedTasks == nil || len(resumeResult.ResumedTasks) != lifecycleTaskCount {
		t.Fatalf("resumed tasks = %v, want %d", resumeResult.ResumedTasks, lifecycleTaskCount)
	}
	if !strings.Contains(resumeResult.Message, "Resumed") {
		t.Fatalf("resume message = %q, want resume notice", resumeResult.Message)
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load final index: %v", err)
	}
	for _, task := range finalIdx.Tasks {
		if task.State != index.TaskStateOpen {
			t.Fatalf("task %q final state = %q, want %q", task.ID, task.State, index.TaskStateOpen)
		}
		if task.Attempts.Total != 2 {
			t.Fatalf("task %q attempts = %d, want %d", task.ID, task.Attempts.Total, 2)
		}
	}

	assertAuditContains(t, repoRoot, "event=worker.timeout")
	assertAuditContains(t, repoRoot, "event=task.transition from=blocked to=open")
}

func setupLifecycleRepo(t *testing.T, workerCommand []string, timeoutSeconds int) *testrepos.TempRepo {
	t.Helper()
	repo := testrepos.New(t)
	if err := config.InitFullLayout(repo.Root); err != nil {
		t.Fatalf("init layout: %v", err)
	}

	governator := filepath.Join(repo.Root, "GOVERNATOR.md")
	if err := os.WriteFile(governator, []byte("# Lifecycle fixture\n"), 0o644); err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}

	writeRolePrompt(t, repo.Root, "worker")
	writeRolePrompt(t, repo.Root, "tester")
	writeRolePrompt(t, repo.Root, "reviewer")
	writeLifecycleConfig(t, repo.Root, workerCommand, timeoutSeconds)

	repo.RunGit(t, "add", "GOVERNATOR.md")
	repo.RunGit(t, "add", filepath.Join("_governator", "config", "config.json"))
	repo.RunGit(t, "add", filepath.Join("_governator", "roles", "worker.md"))
	repo.RunGit(t, "add", filepath.Join("_governator", "roles", "tester.md"))
	repo.RunGit(t, "add", filepath.Join("_governator", "roles", "reviewer.md"))
	repo.RunGit(t, "commit", "-m", "Initialize lifecycle fixture")
	repo.RunGit(t, "remote", "add", "origin", repo.Root)

	return repo
}

func writeRolePrompt(t *testing.T, repoRoot, role string) {
	t.Helper()
	promptPath := filepath.Join(repoRoot, "_governator", "roles", fmt.Sprintf("%s.md", role))
	content := fmt.Sprintf("# %s role agent\n", role)
	if err := os.WriteFile(promptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s role prompt: %v", role, err)
	}
}

func writeLifecycleConfig(t *testing.T, repoRoot string, workerCommand []string, timeoutSeconds int) {
	t.Helper()
	cfg := config.Defaults()
	cfg.Workers.Commands.Default = append([]string(nil), workerCommand...)
	if timeoutSeconds > 0 {
		cfg.Timeouts.WorkerSeconds = timeoutSeconds
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configPath := filepath.Join(repoRoot, "_governator", "config", "config.json")
	if err := os.WriteFile(configPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func assertAuditContains(t *testing.T, repoRoot, substring string) {
	t.Helper()
	auditPath := filepath.Join(repoRoot, "_governator", "_local_state", "audit.log")
	data, err := os.ReadFile(auditPath)
	if err != nil {
		t.Fatalf("read audit log: %v", err)
	}
	if !strings.Contains(string(data), substring) {
		t.Fatalf("audit log missing %q", substring)
	}
}

func lifecycleMarkerForStage(stage string) string {
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

func TestLifecyclePlannerHelper(t *testing.T) {
	if os.Getenv("GO_LIFECYCLE_PLANNER_HELPER") != "1" {
		return
	}
	t.Helper()
	if _, err := os.Stat(os.Args[len(os.Args)-1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprintln(os.Stdout, lifecyclePlannerOutput)
	os.Exit(0)
}

func TestLifecycleWorkerHelper(t *testing.T) {
	if os.Getenv("GO_LIFECYCLE_WORKER_HELPER") != "1" {
		return
	}
	t.Helper()
	mode := os.Getenv("GO_LIFECYCLE_WORKER_MODE")
	if mode == "timeout" {
		time.Sleep(3 * time.Second)
		os.Exit(0)
	}
	stage := os.Getenv("GOVERNATOR_STAGE")
	marker := lifecycleMarkerForStage(stage)
	if marker == "" {
		fmt.Fprintf(os.Stderr, "unsupported stage %q\n", stage)
		os.Exit(2)
	}
	markerPath := filepath.Join("_governator", "_local_state", marker)
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := os.WriteFile(markerPath, []byte("lifecycle marker\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}
