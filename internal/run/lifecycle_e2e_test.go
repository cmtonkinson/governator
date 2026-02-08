// Tests for lifecycle-oriented end-to-end flows covering bootstrapped planning, run execution,
// and role-driven stage transitions.
package run

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/testrepos"
	"github.com/cmtonkinson/governator/internal/worktree"
)

const lifecycleTaskCount = 1

func TestLifecycleEndToEndHappyPath(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "success")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 2)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-LIFE-001", "Lifecycle integration task", "worker")
	task := newTestTask("T-LIFE-001", "Lifecycle integration task", "worker", taskPath, 10)
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add lifecycle plan outputs")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	expectedTasks := lifecycleTaskCount + len(mergedPlanningTasks(t, repoRoot))
	if len(idx.Tasks) != expectedTasks {
		t.Fatalf("index contains %d tasks, want %d", len(idx.Tasks), expectedTasks)
	}

	if err := prepareWorkedTask(t, repoRoot, &idx, repo, config.Defaults().Branches.Base); err != nil {
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

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	worktreePath, err := manager.WorktreePath("T-LIFE-001")
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}
	waitForExitStatus(t, worktreePath, "T-LIFE-001", roles.StageTest)

	runStdout.Reset()
	runStderr.Reset()
	result, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run collect test failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}
	waitForExitStatus(t, worktreePath, "T-LIFE-001", roles.StageReview)

	runStdout.Reset()
	runStderr.Reset()
	result, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run collect review failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}
	if !strings.Contains(result.Message, "review task(s)") {
		t.Fatalf("result message = %q, want review stage summary", result.Message)
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	for _, task := range finalIdx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State != index.TaskStateMerged {
			t.Fatalf("task %q role=%q state = %q, want %q; stdout=%q stderr=%q",
				task.ID, task.Role, task.State, index.TaskStateMerged, runStdout.String(), runStderr.String())
		}
		markerLine := fmt.Sprintf("task_id=%s role=%s event=task.transition from=implemented to=tested", task.ID, task.Role)
		assertAuditContains(t, repoRoot, markerLine)
		doneLine := fmt.Sprintf("task_id=%s role=%s event=task.transition from=tested to=reviewed", task.ID, task.Role)
		assertAuditContains(t, repoRoot, doneLine)
	}

	assertAuditContains(t, repoRoot, "event=task.transition from=implemented to=tested")
	assertAuditContains(t, repoRoot, "event=task.transition from=tested to=reviewed")
}

func TestLifecycleEndToEndTimeoutResume(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "timeout")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 1)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-LIFE-001", "Lifecycle integration task", "worker")
	task := newTestTask("T-LIFE-001", "Lifecycle integration task", "worker", taskPath, 10)
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add lifecycle plan outputs")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	expectedTasks := lifecycleTaskCount + len(mergedPlanningTasks(t, repoRoot))
	if len(idx.Tasks) != expectedTasks {
		t.Fatalf("index contains %d tasks, want %d", len(idx.Tasks), expectedTasks)
	}

	if err := prepareWorkedTask(t, repoRoot, &idx, repo, config.Defaults().Branches.Base); err != nil {
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

	time.Sleep(2 * time.Second)
	timeoutStdout.Reset()
	timeoutStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &timeoutStdout, Stderr: &timeoutStderr}); err != nil {
		t.Fatalf("second run (timeout collect) failed: %v", err)
	}

	timeoutIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index after timeout: %v", err)
	}
	for _, task := range timeoutIdx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
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

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	worktreePath, err := manager.WorktreePath("T-LIFE-001")
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}
	waitForExitStatus(t, worktreePath, "T-LIFE-001", roles.StageWork)
	resumeStdout.Reset()
	resumeStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &resumeStdout, Stderr: &resumeStderr}); err != nil {
		t.Fatalf("third run (collect work) failed: %v", err)
	}

	waitForExitStatus(t, worktreePath, "T-LIFE-001", roles.StageTest)
	resumeStdout.Reset()
	resumeStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &resumeStdout, Stderr: &resumeStderr}); err != nil {
		t.Fatalf("fourth run (collect test) failed: %v", err)
	}
	waitForExitStatus(t, worktreePath, "T-LIFE-001", roles.StageReview)
	resumeStdout.Reset()
	resumeStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &resumeStdout, Stderr: &resumeStderr}); err != nil {
		t.Fatalf("fifth run (collect review) failed: %v", err)
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load final index: %v", err)
	}
	for _, task := range finalIdx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State != index.TaskStateMerged {
			t.Fatalf("task %q final state = %q, want %q (stdout=%q stderr=%q)", task.ID, task.State, index.TaskStateMerged, resumeStdout.String(), resumeStderr.String())
		}
		if task.Attempts.Total != 2 {
			t.Fatalf("task %q attempts = %d, want %d", task.ID, task.Attempts.Total, 2)
		}
	}

	assertAuditContains(t, repoRoot, "event=worker.timeout")
	assertAuditContains(t, repoRoot, "event=task.transition from=blocked to=triaged")
}

// TestLifecycleEndToEndWorkerNonZeroExit tests worker failure during work stage.
// Coverage: Triaged → Blocked (work stage failure)
func TestLifecycleEndToEndWorkerNonZeroExit(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "fail")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 10)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-FAIL-001", "Worker failure task", "worker")
	task := newTestTask("T-FAIL-001", "Worker failure task", "worker", taskPath, 10)
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add worker failure test task")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")

	// First run: worker dispatched and fails (task starts in open/triaged state)
	// The work stage will automatically create the worktree when dispatching
	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	_, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	worktreePath, err := manager.WorktreePath("T-FAIL-001")
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}

	// Wait for worker to exit
	waitForExitStatus(t, worktreePath, "T-FAIL-001", roles.StageWork)

	// Second run: collect worker failure
	runStdout.Reset()
	runStderr.Reset()
	_, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run collect failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	// Verify task transitioned to blocked
	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	for _, task := range finalIdx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State != index.TaskStateBlocked {
			t.Fatalf("task %q state = %q, want %q; stdout=%q stderr=%q",
				task.ID, task.State, index.TaskStateBlocked, runStdout.String(), runStderr.String())
		}
		if task.Attempts.Failed != 1 {
			t.Fatalf("task %q failed attempts = %d, want 1", task.ID, task.Attempts.Failed)
		}
		// Note: Total is not incremented on first failure, only on resume
		if task.Attempts.Total != 0 {
			t.Fatalf("task %q total attempts = %d, want 0", task.ID, task.Attempts.Total)
		}
	}

	// Verify audit log contains agent outcome with exit code and state transition
	assertAuditContains(t, repoRoot, "event=agent.outcome")
	assertAuditContains(t, repoRoot, "exit_code=1")
	assertAuditContains(t, repoRoot, "event=task.transition from=triaged to=blocked")

	// Verify no marker file was created
	markerPath := filepath.Join(worktreePath, "_governator", "_local-state", "worked.md")
	if _, err := os.Stat(markerPath); err == nil {
		t.Fatalf("marker file %s should not exist after worker failure", markerPath)
	}
}

// TestLifecycleEndToEndTestStageFailure tests worker failure during test stage.
// Coverage: Implemented → Blocked (test stage failure)
func TestLifecycleEndToEndTestStageFailure(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "fail")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 10)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-TEST-FAIL-001", "Test failure task", "worker")
	task := newTestTask("T-TEST-FAIL-001", "Test failure task", "worker", taskPath, 10)
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add test failure task")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}

	// Prepare task in implemented state (with worked.md committed)
	if err := prepareWorkedTask(t, repoRoot, &idx, repo, config.Defaults().Branches.Base); err != nil {
		t.Fatalf("prepare worked tasks: %v", err)
	}
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save prepared index: %v", err)
	}

	// First run: test worker dispatched and fails
	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	_, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	worktreePath, err := manager.WorktreePath("T-TEST-FAIL-001")
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}

	// Wait for test worker to exit
	waitForExitStatus(t, worktreePath, "T-TEST-FAIL-001", roles.StageTest)

	// Second run: collect test worker failure
	runStdout.Reset()
	runStderr.Reset()
	_, err = Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr})
	if err != nil {
		t.Fatalf("run.Run collect failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	// Verify task transitioned to blocked
	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	for _, task := range finalIdx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State != index.TaskStateBlocked {
			t.Fatalf("task %q state = %q, want %q; stdout=%q stderr=%q",
				task.ID, task.State, index.TaskStateBlocked, runStdout.String(), runStderr.String())
		}
		if task.BlockedReason == "" {
			t.Fatalf("task %q blocked reason should be set", task.ID)
		}
	}

	// Verify audit log contains agent outcome with exit code and state transition
	assertAuditContains(t, repoRoot, "event=agent.outcome")
	assertAuditContains(t, repoRoot, "exit_code=1")
	assertAuditContains(t, repoRoot, "event=task.transition from=implemented to=blocked")
}

// TestLifecycleEndToEndReviewStageFailure tests worker failure during review stage.
// Coverage: Tested → Blocked (review stage failure)
func TestLifecycleEndToEndReviewStageFailure(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	// Use fail-on-review mode: succeeds for work/test, fails for review
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "fail-on-review")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 10)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-REVIEW-FAIL-001", "Review failure task", "worker")
	task := newTestTask("T-REVIEW-FAIL-001", "Review failure task", "worker", taskPath, 10)
	task.Retries.MaxAttempts = 1
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add review failure task")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}

	// Prepare task in implemented state
	if err := prepareWorkedTask(t, repoRoot, &idx, repo, config.Defaults().Branches.Base); err != nil {
		t.Fatalf("prepare worked tasks: %v", err)
	}
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save prepared index: %v", err)
	}

	// First run: test stage (will succeed with fail-on-review mode)
	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run test stage failed: %v", err)
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	worktreePath, err := manager.WorktreePath("T-REVIEW-FAIL-001")
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}

	// Wait for test stage to complete
	waitForExitStatus(t, worktreePath, "T-REVIEW-FAIL-001", roles.StageTest)

	// Second run: collect test results and dispatch review (which will fail)
	runStdout.Reset()
	runStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run collect test failed: %v", err)
	}

	// Wait for review stage to complete (it will fail)
	waitForExitStatus(t, worktreePath, "T-REVIEW-FAIL-001", roles.StageReview)

	// Third run: collect review failure
	runStdout.Reset()
	runStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run collect review failed: %v", err)
	}

	// Verify task transitioned to blocked
	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	for _, task := range finalIdx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State != index.TaskStateTriaged {
			t.Fatalf("task %q state = %q, want %q; stdout=%q stderr=%q",
				task.ID, task.State, index.TaskStateTriaged, runStdout.String(), runStderr.String())
		}
	}

	// Verify audit log contains agent outcome with exit code and state transition
	assertAuditContains(t, repoRoot, "event=agent.outcome")
	assertAuditContains(t, repoRoot, "exit_code=1")
	assertAuditContains(t, repoRoot, "event=task.transition from=tested to=triaged")
}

// TestLifecycleEndToEndRetryLimitExhausted ensures blocked tasks stop resuming once retries are exhausted.
func TestLifecycleEndToEndRetryLimitExhausted(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "fail")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 10)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-RETRY-001", "Retry exhaustion task", "worker")
	task := newTestTask("T-RETRY-001", "Retry exhaustion task", "worker", taskPath, 10)
	task.Retries.MaxAttempts = 2
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add retry exhaustion task")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	worktreePath, err := manager.WorktreePath(task.ID)
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}

	expectedFailed := 0
	for attempt := 0; attempt < 3; attempt++ {
		var runStdout bytes.Buffer
		var runStderr bytes.Buffer
		if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
			t.Fatalf("run.Run dispatch failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
		}
		waitForExitStatus(t, worktreePath, task.ID, roles.StageWork)

		runStdout.Reset()
		runStderr.Reset()
		if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
			t.Fatalf("run.Run collect failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
		}

		idx, err := index.Load(indexPath)
		if err != nil {
			t.Fatalf("load index after attempt %d: %v", attempt+1, err)
		}
		executionTask := findExecutionTask(t, idx, task.ID)
		expectedFailed++
		if executionTask.State != index.TaskStateBlocked {
			t.Fatalf("task %q state = %q after attempt %d, want %q", executionTask.ID, executionTask.State, attempt+1, index.TaskStateBlocked)
		}
		if executionTask.Attempts.Failed != expectedFailed {
			t.Fatalf("task %q failed attempts = %d, want %d", executionTask.ID, executionTask.Attempts.Failed, expectedFailed)
		}
		if executionTask.Attempts.Total != attempt {
			t.Fatalf("task %q total attempts = %d, want %d", executionTask.ID, executionTask.Attempts.Total, attempt)
		}
	}

	var limitStdout bytes.Buffer
	var limitStderr bytes.Buffer
	result, err := Run(repoRoot, Options{Stdout: &limitStdout, Stderr: &limitStderr})
	if err != nil {
		t.Fatalf("run.Run retry limit failed: %v, stdout=%q, stderr=%q", err, limitStdout.String(), limitStderr.String())
	}
	if !strings.Contains(result.Message, "retry limit") {
		t.Fatalf("result message = %q, want retry limit notice", result.Message)
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index after retry limit: %v", err)
	}
	finalTask := findExecutionTask(t, finalIdx, task.ID)
	if finalTask.State != index.TaskStateBlocked {
		t.Fatalf("task %q final state = %q, want %q", finalTask.ID, finalTask.State, index.TaskStateBlocked)
	}
	if finalTask.Attempts.Total != 2 {
		t.Fatalf("task %q total attempts = %d, want 2", finalTask.ID, finalTask.Attempts.Total)
	}
	if finalTask.Attempts.Failed != 3 {
		t.Fatalf("task %q failed attempts = %d, want 3", finalTask.ID, finalTask.Attempts.Failed)
	}
}

// TestLifecycleEndToEndMergeConflictDuringReview ensures merge conflicts route tasks to conflict state.
func TestLifecycleEndToEndMergeConflictDuringReview(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "success")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	fixture := setupLifecycleConflictFixture(t, workerCommand)

	runStderr := driveTaskToConflict(t, fixture.repoRoot, fixture.task.ID, fixture.worktreePath)

	finalIdx, err := index.Load(fixture.indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	finalTask := findExecutionTask(t, finalIdx, fixture.task.ID)
	if finalTask.State != index.TaskStateConflict {
		t.Fatalf("task %q final state = %q, want %q; stderr=%q", finalTask.ID, finalTask.State, index.TaskStateConflict, runStderr)
	}
	if !finalTask.MergeConflict {
		t.Fatalf("task %q merge conflict flag = %v, want true", finalTask.ID, finalTask.MergeConflict)
	}
}

// TestLifecycleEndToEndConflictResolutionSuccess ensures conflicts can be resolved and merged.
func TestLifecycleEndToEndConflictResolutionSuccess(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "resolve")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	fixture := setupLifecycleConflictFixture(t, workerCommand)

	_ = driveTaskToConflict(t, fixture.repoRoot, fixture.task.ID, fixture.worktreePath)
	waitForExitStatus(t, fixture.worktreePath, fixture.task.ID, roles.StageResolve)
	collectConflictResolution(t, fixture.repoRoot, fixture.task.ID, fixture.worktreePath)

	finalIdx, err := index.Load(fixture.indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	finalTask := findExecutionTask(t, finalIdx, fixture.task.ID)
	if finalTask.State != index.TaskStateMerged {
		t.Fatalf("task %q final state = %q, want %q", finalTask.ID, finalTask.State, index.TaskStateMerged)
	}
}

// TestLifecycleEndToEndConflictResolutionFailure ensures failed resolution blocks the task.
func TestLifecycleEndToEndConflictResolutionFailure(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "fail-on-resolve")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	fixture := setupLifecycleConflictFixture(t, workerCommand)

	_ = driveTaskToConflict(t, fixture.repoRoot, fixture.task.ID, fixture.worktreePath)
	waitForExitStatus(t, fixture.worktreePath, fixture.task.ID, roles.StageResolve)
	collectConflictResolution(t, fixture.repoRoot, fixture.task.ID, fixture.worktreePath)

	finalIdx, err := index.Load(fixture.indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	finalTask := findExecutionTask(t, finalIdx, fixture.task.ID)
	if finalTask.State != index.TaskStateBlocked {
		t.Fatalf("task %q final state = %q, want %q", finalTask.ID, finalTask.State, index.TaskStateBlocked)
	}
	if finalTask.BlockedReason == "" {
		t.Fatalf("task %q blocked reason should be set", finalTask.ID)
	}
}

// TestLifecycleEndToEndConflictResolutionReConflict ensures a new conflict returns tasks to conflict state.
func TestLifecycleEndToEndConflictResolutionReConflict(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "resolve")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	fixture := setupLifecycleConflictFixture(t, workerCommand)

	_ = driveTaskToConflict(t, fixture.repoRoot, fixture.task.ID, fixture.worktreePath)
	waitForExitStatus(t, fixture.worktreePath, fixture.task.ID, roles.StageResolve)
	createConflictingCommit(t, fixture.repo, fixture.conflictFile, "new conflicting main change\n")
	collectConflictResolution(t, fixture.repoRoot, fixture.task.ID, fixture.worktreePath)

	finalIdx, err := index.Load(fixture.indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	finalTask := findExecutionTask(t, finalIdx, fixture.task.ID)
	if finalTask.State != index.TaskStateConflict {
		t.Fatalf("task %q final state = %q, want %q", finalTask.ID, finalTask.State, index.TaskStateConflict)
	}
	if !finalTask.MergeConflict {
		t.Fatalf("task %q merge conflict flag = %v, want true", finalTask.ID, finalTask.MergeConflict)
	}
}

// TestLifecycleEndToEndMultipleTasksProgressingConcurrently validates mixed-stage dispatch and collection.
func TestLifecycleEndToEndMultipleTasksProgressingConcurrently(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "success")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 10)
	repoRoot := repo.Root

	cfg := config.Defaults()
	cfg.Workers.Commands.Default = append([]string(nil), workerCommand...)
	cfg.Concurrency.Global = 4
	cfg.Concurrency.DefaultRole = 4
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	configPath := filepath.Join(repoRoot, "_governator", "_durable-state", "config.json")
	if err := os.WriteFile(configPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	t1Path := writeTestTaskFile(t, repoRoot, "T-MULTI-001", "Multi work task", "worker")
	t2Path := writeTestTaskFile(t, repoRoot, "T-MULTI-002", "Multi test task", "worker")
	t3Path := writeTestTaskFile(t, repoRoot, "T-MULTI-003", "Multi review task", "worker")
	t4Path := writeTestTaskFile(t, repoRoot, "T-MULTI-004", "Multi conflict task", "worker")

	t1 := newTestTask("T-MULTI-001", "Multi work task", "worker", t1Path, 10)
	t2 := newTestTask("T-MULTI-002", "Multi test task", "worker", t2Path, 20)
	t3 := newTestTask("T-MULTI-003", "Multi review task", "worker", t3Path, 30)
	t4 := newTestTask("T-MULTI-004", "Multi conflict task", "worker", t4Path, 40)

	t2.State = index.TaskStateImplemented
	t3.State = index.TaskStateTested
	t4.State = index.TaskStateConflict
	t4.MergeConflict = true

	writeTestTaskIndex(t, repoRoot, []index.Task{t1, t2, t3, t4})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add multi-stage tasks")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}

	baseBranch := config.Defaults().Branches.Base
	t2PathWorktree := ensureTaskWorktree(t, repoRoot, t2, baseBranch)
	t3PathWorktree := ensureTaskWorktree(t, repoRoot, t3, baseBranch)
	t4PathWorktree := ensureTaskWorktree(t, repoRoot, t4, baseBranch)

	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run dispatch failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	t1Worktree, err := manager.WorktreePath(t1.ID)
	if err != nil {
		t.Fatalf("worktree path: %v", err)
	}

	waitForExitStatus(t, t1Worktree, t1.ID, roles.StageWork)
	waitForExitStatus(t, t2PathWorktree, t2.ID, roles.StageTest)
	waitForExitStatus(t, t3PathWorktree, t3.ID, roles.StageReview)
	waitForExitStatus(t, t4PathWorktree, t4.ID, roles.StageResolve)

	runStdout.Reset()
	runStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run collect failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}

	if task := findExecutionTask(t, finalIdx, t1.ID); task.State != index.TaskStateImplemented {
		t.Fatalf("task %q state = %q, want %q", task.ID, task.State, index.TaskStateImplemented)
	}
	if task := findExecutionTask(t, finalIdx, t2.ID); task.State != index.TaskStateTested {
		t.Fatalf("task %q state = %q, want %q", task.ID, task.State, index.TaskStateTested)
	}
	if task := findExecutionTask(t, finalIdx, t3.ID); task.State != index.TaskStateMerged {
		t.Fatalf("task %q state = %q, want %q", task.ID, task.State, index.TaskStateMerged)
	}
	if task := findExecutionTask(t, finalIdx, t4.ID); task.State != index.TaskStateMerged {
		t.Fatalf("task %q state = %q, want %q", task.ID, task.State, index.TaskStateMerged)
	}
}

// TestLifecycleEndToEndWorkerTimeoutWithRetry validates timeout handling across retries.
func TestLifecycleEndToEndWorkerTimeoutWithRetry(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "timeout")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 1)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-TIMEOUT-001", "Timeout retry task", "worker")
	task := newTestTask("T-TIMEOUT-001", "Timeout retry task", "worker", taskPath, 10)
	task.Retries.MaxAttempts = 1
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add timeout retry task")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")

	if _, err := Run(repoRoot, Options{Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatalf("run.Run dispatch timeout failed: %v", err)
	}
	time.Sleep(2 * time.Second)
	if _, err := Run(repoRoot, Options{Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatalf("run.Run collect timeout failed: %v", err)
	}

	timeoutIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index after timeout: %v", err)
	}
	timeoutTask := findExecutionTask(t, timeoutIdx, task.ID)
	if timeoutTask.State != index.TaskStateBlocked {
		t.Fatalf("task %q state = %q, want %q", timeoutTask.ID, timeoutTask.State, index.TaskStateBlocked)
	}

	if _, err := Run(repoRoot, Options{Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatalf("run.Run resume timeout failed: %v", err)
	}
	time.Sleep(2 * time.Second)
	if _, err := Run(repoRoot, Options{Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatalf("run.Run collect timeout retry failed: %v", err)
	}

	retryIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index after timeout retry: %v", err)
	}
	retryTask := findExecutionTask(t, retryIdx, task.ID)
	if retryTask.State != index.TaskStateBlocked {
		t.Fatalf("task %q state = %q, want %q", retryTask.ID, retryTask.State, index.TaskStateBlocked)
	}
	if retryTask.Attempts.Total != 1 {
		t.Fatalf("task %q total attempts = %d, want 1", retryTask.ID, retryTask.Attempts.Total)
	}

	result, err := Run(repoRoot, Options{Stdout: io.Discard, Stderr: io.Discard})
	if err != nil {
		t.Fatalf("run.Run retry limit check failed: %v", err)
	}
	if !strings.Contains(result.Message, "retry limit") {
		t.Fatalf("result message = %q, want retry limit notice", result.Message)
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index after retry limit: %v", err)
	}
	finalTask := findExecutionTask(t, finalIdx, task.ID)
	if finalTask.Attempts.Total != 1 {
		t.Fatalf("task %q total attempts = %d, want 1", finalTask.ID, finalTask.Attempts.Total)
	}
}

// TestLifecycleEndToEndMergeStageForResolvedTask ensures resolved tasks merge cleanly.
func TestLifecycleEndToEndMergeStageForResolvedTask(t *testing.T) {
	t.Setenv("GO_LIFECYCLE_WORKER_HELPER", "1")
	t.Setenv("GO_LIFECYCLE_WORKER_MODE", "success")

	workerCommand := []string{os.Args[0], "-test.run=TestLifecycleWorkerHelper", "--", "{task_path}"}
	repo := setupLifecycleRepo(t, workerCommand, 10)
	repoRoot := repo.Root

	taskPath := writeTestTaskFile(t, repoRoot, "T-RESOLVED-001", "Resolved merge task", "worker")
	task := newTestTask("T-RESOLVED-001", "Resolved merge task", "worker", taskPath, 10)
	task.State = index.TaskStateResolved
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add resolved task")

	baseBranch := config.Defaults().Branches.Base
	worktreePath := ensureTaskWorktree(t, repoRoot, task, baseBranch)
	commitFileInWorktree(t, worktreePath, "resolved.txt", "resolved content\n", "Resolved task content")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	if _, err := Run(repoRoot, Options{Stdout: io.Discard, Stderr: io.Discard}); err != nil {
		t.Fatalf("run.Run merge resolved failed: %v", err)
	}

	finalIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}
	finalTask := findExecutionTask(t, finalIdx, task.ID)
	if finalTask.State != index.TaskStateMerged {
		t.Fatalf("task %q final state = %q, want %q", finalTask.ID, finalTask.State, index.TaskStateMerged)
	}
}

func setupLifecycleRepo(t *testing.T, workerCommand []string, timeoutSeconds int) *testrepos.TempRepo {
	t.Helper()
	repo := testrepos.New(t)
	if err := config.InitFullLayout(repo.Root, config.InitOptions{}); err != nil {
		t.Fatalf("init layout: %v", err)
	}

	governator := filepath.Join(repo.Root, "GOVERNATOR.md")
	if err := os.WriteFile(governator, []byte("# Lifecycle fixture\n"), 0o644); err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}

	writeRolePrompt(t, repo.Root, "worker")
	writeRolePrompt(t, repo.Root, "tester")
	writeRolePrompt(t, repo.Root, "reviewer")
	writeRoleAssignmentPrompt(t, repo.Root)
	writeLifecycleConfig(t, repo.Root, workerCommand, timeoutSeconds)

	repo.RunGit(t, "add", "GOVERNATOR.md")
	repo.RunGit(t, "add", filepath.Join("_governator", "_durable-state", "config.json"))
	repo.RunGit(t, "add", filepath.Join("_governator", "roles", "worker.md"))
	repo.RunGit(t, "add", filepath.Join("_governator", "roles", "tester.md"))
	repo.RunGit(t, "add", filepath.Join("_governator", "roles", "reviewer.md"))
	repo.RunGit(t, "add", filepath.Join("_governator", "prompts", "role-assignment.md"))
	repo.RunGit(t, "commit", "-m", "Initialize lifecycle fixture")

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

// writeRoleAssignmentPrompt creates the conflict resolution role assignment prompt fixture.
func writeRoleAssignmentPrompt(t *testing.T, repoRoot string) {
	t.Helper()
	promptDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := os.MkdirAll(promptDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts dir: %v", err)
	}
	promptPath := filepath.Join(promptDir, "role-assignment.md")
	content := "# Role Assignment\nSelect the best role for conflict resolution.\n"
	if err := os.WriteFile(promptPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write role assignment prompt: %v", err)
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
	configPath := filepath.Join(repoRoot, "_governator", "_durable-state", "config.json")
	if err := os.WriteFile(configPath, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func assertAuditContains(t *testing.T, repoRoot, substring string) {
	t.Helper()
	auditPath := filepath.Join(repoRoot, "_governator", "_local-state", "audit.log")
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

func TestLifecycleWorkerHelper(t *testing.T) {
	if os.Getenv("GO_LIFECYCLE_WORKER_HELPER") != "1" {
		return
	}
	t.Helper()
	mode := os.Getenv("GO_LIFECYCLE_WORKER_MODE")

	// Handle different worker modes
	switch mode {
	case "timeout":
		time.Sleep(3 * time.Second)
		os.Exit(0)
	case "fail":
		// Exit with non-zero code without creating marker
		os.Exit(1)
	case "sleep":
		// Sleep then exit successfully (for timeout tests)
		time.Sleep(3 * time.Second)
		os.Exit(0)
	case "fail-on-review":
		// Succeed for work and test, fail for review
		stage := os.Getenv("GOVERNATOR_STAGE")
		if stage == "review" {
			os.Exit(1)
		}
		// Fall through to normal marker creation for other stages
	case "fail-on-resolve":
		stage := os.Getenv("GOVERNATOR_STAGE")
		if stage == "resolve" {
			os.Exit(1)
		}
		// Fall through to normal marker creation for other stages
	case "resolve":
		stage := os.Getenv("GOVERNATOR_STAGE")
		if stage == "resolve" {
			if err := resolveLifecycleConflict(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(2)
			}
		}
		// Fall through to normal marker creation for other stages
	case "success":
		// Fall through to normal marker creation
	default:
		// Default to success mode
	}

	stage := os.Getenv("GOVERNATOR_STAGE")
	marker := lifecycleMarkerForStage(stage)
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
	if err := os.WriteFile(markerPath, []byte("lifecycle marker\n"), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if err := runLifecycleGitCommand(markerPath); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	os.Exit(0)
}

func runLifecycleGitCommand(path string) error {
	if err := exec.Command("git", "add", path).Run(); err != nil {
		return fmt.Errorf("git add failed: %w", err)
	}
	diffCmd := exec.Command("git", "diff", "--cached", "--quiet")
	if err := diffCmd.Run(); err == nil {
		return nil
	} else if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		if err := exec.Command("git", "commit", "-m", "Lifecycle work stage").Run(); err != nil {
			return fmt.Errorf("git commit failed: %w", err)
		}
		return nil
	} else if err != nil {
		return fmt.Errorf("git diff failed: %w", err)
	}
	return nil
}

// lifecycleConflictFixture captures shared state for conflict-oriented lifecycle tests.
type lifecycleConflictFixture struct {
	repo         *testrepos.TempRepo
	repoRoot     string
	indexPath    string
	task         index.Task
	worktreePath string
	conflictFile string
}

// setupLifecycleConflictFixture seeds a task branch and main branch with a merge conflict.
func setupLifecycleConflictFixture(t *testing.T, workerCommand []string) lifecycleConflictFixture {
	t.Helper()
	repo := setupLifecycleRepo(t, workerCommand, 10)
	repoRoot := repo.Root

	conflictFile := "shared.txt"
	commitFileInRepo(t, repo, conflictFile, "base content\n", "Add shared file")

	taskPath := writeTestTaskFile(t, repoRoot, "T-CONFLICT-001", "Merge conflict task", "worker")
	task := newTestTask("T-CONFLICT-001", "Merge conflict task", "worker", taskPath, 10)
	writeTestTaskIndex(t, repoRoot, []index.Task{task})

	repo.RunGit(t, "add", filepath.Join("_governator", "tasks"))
	repo.RunGit(t, "commit", "-m", "Add conflict task")

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	for i := range idx.Tasks {
		if idx.Tasks[i].ID == task.ID {
			idx.Tasks[i].State = index.TaskStateImplemented
			idx.Tasks[i].Attempts.Total = 1
			break
		}
	}
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save prepared index: %v", err)
	}

	baseBranch := config.Defaults().Branches.Base
	worktreePath := ensureTaskWorktree(t, repoRoot, task, baseBranch)
	commitFileInWorktree(t, worktreePath, conflictFile, "task change\n", "Task change")
	createConflictingCommit(t, repo, conflictFile, "main change\n")

	return lifecycleConflictFixture{
		repo:         repo,
		repoRoot:     repoRoot,
		indexPath:    indexPath,
		task:         task,
		worktreePath: worktreePath,
		conflictFile: conflictFile,
	}
}

// driveTaskToConflict runs test and review stages until merge conflict is detected.
func driveTaskToConflict(t *testing.T, repoRoot, taskID, worktreePath string) string {
	t.Helper()
	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run dispatch test failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}
	waitForExitStatus(t, worktreePath, taskID, roles.StageTest)

	runStdout.Reset()
	runStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run collect test failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}
	waitForExitStatus(t, worktreePath, taskID, roles.StageReview)

	runStdout.Reset()
	runStderr.Reset()
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run collect review failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}
	return runStderr.String()
}

// collectConflictResolution collects conflict resolution results and runs merge stage.
func collectConflictResolution(t *testing.T, repoRoot, taskID, worktreePath string) {
	t.Helper()
	var runStdout bytes.Buffer
	var runStderr bytes.Buffer
	if _, err := Run(repoRoot, Options{Stdout: &runStdout, Stderr: &runStderr}); err != nil {
		t.Fatalf("run.Run collect resolve failed: %v, stdout=%q, stderr=%q", err, runStdout.String(), runStderr.String())
	}
}

// ensureTaskWorktree guarantees a worktree exists for the task and returns its path.
func ensureTaskWorktree(t *testing.T, repoRoot string, task index.Task, baseBranch string) string {
	t.Helper()
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("worktree manager: %v", err)
	}
	branchName := TaskBranchName(task)
	if strings.TrimSpace(baseBranch) == "" {
		baseBranch = config.Defaults().Branches.Base
	}
	result, err := manager.EnsureWorktree(worktree.Spec{
		WorkstreamID: task.ID,
		Branch:       branchName,
		BaseBranch:   baseBranch,
	})
	if err != nil {
		t.Fatalf("ensure worktree: %v", err)
	}
	return result.Path
}

// commitFileInWorktree writes a file in the worktree and commits it.
func commitFileInWorktree(t *testing.T, worktreePath, relativePath, content, message string) {
	t.Helper()
	fullPath := filepath.Join(worktreePath, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", fullPath, err)
	}
	if _, err := execGitCommand(worktreePath, "add", relativePath); err != nil {
		t.Fatalf("git add %s: %v", relativePath, err)
	}
	if _, err := execGitCommand(worktreePath, "commit", "-m", message); err != nil {
		t.Fatalf("git commit %s: %v", relativePath, err)
	}
}

// commitFileInRepo writes a file in the repo root and commits it.
func commitFileInRepo(t *testing.T, repo *testrepos.TempRepo, relativePath, content, message string) {
	t.Helper()
	fullPath := filepath.Join(repo.Root, relativePath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", fullPath, err)
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", fullPath, err)
	}
	repo.RunGit(t, "add", relativePath)
	repo.RunGit(t, "commit", "-m", message)
}

// createConflictingCommit adds a conflicting change on the main branch.
func createConflictingCommit(t *testing.T, repo *testrepos.TempRepo, filePath, content string) {
	t.Helper()
	repo.RunGit(t, "checkout", "main")
	commitFileInRepo(t, repo, filePath, content, "Conflicting change on main")
}

// findExecutionTask returns the execution task by ID or fails the test.
func findExecutionTask(t *testing.T, idx index.Index, taskID string) index.Task {
	t.Helper()
	for _, task := range idx.Tasks {
		if task.ID == taskID && task.Kind == index.TaskKindExecution {
			return task
		}
	}
	t.Fatalf("execution task %q not found", taskID)
	return index.Task{}
}

// assertFileContent verifies a file matches the expected content.
func assertFileContent(t *testing.T, repoRoot, relativePath, expected string) {
	t.Helper()
	fullPath := filepath.Join(repoRoot, relativePath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("read %s: %v", fullPath, err)
	}
	if string(data) != expected {
		t.Fatalf("file %s content = %q, want %q", relativePath, string(data), expected)
	}
}

// resolveLifecycleConflict rebases on main and resolves conflicts by overwriting conflicted files.
func resolveLifecycleConflict() error {
	const baseBranch = "main"
	if err := runLifecycleGit("rebase", baseBranch); err == nil {
		return nil
	}
	conflicted, err := runLifecycleGitOutput("diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return fmt.Errorf("detect conflicted files: %w", err)
	}
	files := strings.Fields(conflicted)
	if len(files) == 0 {
		return fmt.Errorf("rebase failed but no conflicted files found")
	}
	for _, file := range files {
		if err := os.WriteFile(file, []byte("resolved by lifecycle helper\n"), 0o644); err != nil {
			return fmt.Errorf("write resolved file %s: %w", file, err)
		}
		if err := runLifecycleGit("add", file); err != nil {
			return err
		}
	}
	if err := runLifecycleGit("rebase", "--continue"); err != nil {
		return err
	}
	return nil
}

// runLifecycleGit executes a git command for the lifecycle worker helper.
func runLifecycleGit(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:", "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

// runLifecycleGitOutput executes git and returns stdout for lifecycle worker helpers.
func runLifecycleGitOutput(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Env = append(os.Environ(), "GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:", "GIT_TERMINAL_PROMPT=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
