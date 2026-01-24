// Tests for run orchestrator.
package run

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/phase"
	"github.com/cmtonkinson/governator/internal/testrepos"
	"github.com/cmtonkinson/governator/internal/worktree"
)

// TestRunHappyPathWithResume ensures run command properly handles resume scenarios.
func TestRunHappyPathWithResume(t *testing.T) {
	t.Parallel()
	repoRoot := setupTestRepoWithConfig(t)

	// Create a test index with a blocked task that can be resumed
	idx := index.Index{
		SchemaVersion: 1,
		Digests: index.Digests{
			GovernatorMD: computeTestDigest(),
			PlanningDocs: map[string]string{},
		},
		Tasks: []index.Task{
			{
				ID:    "T-001",
				State: index.TaskStateBlocked,
				Attempts: index.AttemptCounters{
					Total:  1,
					Failed: 1,
				},
				Retries: index.RetryPolicy{
					MaxAttempts: 3,
				},
			},
		},
	}

	// Save the index
	indexPath := filepath.Join(repoRoot, "_governator/task-index.json")
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	// Create a preserved worktree for the task
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}

	worktreePath, err := manager.WorktreePath("T-001", 1)
	if err != nil {
		t.Fatalf("get worktree path: %v", err)
	}

	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create worktree directory: %v", err)
	}

	var stdout, stderr bytes.Buffer
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	result, err := Run(repoRoot, opts)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Check that the task was resumed
	if len(result.ResumedTasks) != 1 {
		t.Fatalf("expected 1 resumed task, got %d", len(result.ResumedTasks))
	}
	if result.ResumedTasks[0] != "T-001" {
		t.Fatalf("resumed task = %q, want %q", result.ResumedTasks[0], "T-001")
	}

	// Check that output was generated
	stdoutStr := stdout.String()
	if !strings.Contains(stdoutStr, "Resuming task T-001") {
		t.Fatalf("stdout should contain resume message, got: %q", stdoutStr)
	}

	// Verify the index was updated
	updatedIdx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load updated index: %v", err)
	}

	task := findTaskByID(t, updatedIdx, "T-001")
	if task.Attempts.Total != 2 {
		t.Fatalf("task attempts total = %d, want %d", task.Attempts.Total, 2)
	}
	if task.State != index.TaskStateBlocked {
		t.Fatalf("task state = %q, want %q", task.State, index.TaskStateBlocked)
	}
}

// TestRunBlocksTasksExceedingRetryLimit ensures tasks exceeding retry limits are blocked.
func TestRunBlocksTasksExceedingRetryLimit(t *testing.T) {
	t.Parallel()
	repoRoot := setupTestRepoWithConfig(t)

	// Create a test index with a blocked task that has exceeded retry limits
	idx := index.Index{
		SchemaVersion: 1,
		Digests: index.Digests{
			GovernatorMD: computeTestDigest(),
			PlanningDocs: map[string]string{},
		},
		Tasks: []index.Task{
			{
				ID:    "T-001",
				State: index.TaskStateBlocked,
				Attempts: index.AttemptCounters{
					Total:  3,
					Failed: 3,
				},
				Retries: index.RetryPolicy{
					MaxAttempts: 3,
				},
			},
		},
	}

	// Save the index
	indexPath := filepath.Join(repoRoot, "_governator/task-index.json")
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	// Create a preserved worktree for the task
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		t.Fatalf("create worktree manager: %v", err)
	}

	worktreePath, err := manager.WorktreePath("T-001", 3)
	if err != nil {
		t.Fatalf("get worktree path: %v", err)
	}

	if err := os.MkdirAll(worktreePath, 0o755); err != nil {
		t.Fatalf("create worktree directory: %v", err)
	}

	var stdout, stderr bytes.Buffer
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	result, err := Run(repoRoot, opts)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Check that the task was blocked
	if len(result.BlockedTasks) != 1 {
		t.Fatalf("expected 1 blocked task, got %d", len(result.BlockedTasks))
	}
	if result.BlockedTasks[0] != "T-001" {
		t.Fatalf("blocked task = %q, want %q", result.BlockedTasks[0], "T-001")
	}

	// Check that output was generated
	stdoutStr := stdout.String()
	if !strings.Contains(stdoutStr, "Task T-001 blocked: exceeded retry limit") {
		t.Fatalf("stdout should contain blocked message, got: %q", stdoutStr)
	}
}

// TestRunNoResumeCandidates ensures run works when there are no resume candidates.
func TestRunNoResumeCandidates(t *testing.T) {
	t.Parallel()
	repoRoot := setupTestRepoWithConfig(t)

	// Create a test index with no blocked tasks
	idx := index.Index{
		SchemaVersion: 1,
		Digests: index.Digests{
			GovernatorMD: computeTestDigest(),
			PlanningDocs: map[string]string{},
		},
		Tasks: []index.Task{
			{
				ID:    "T-001",
				State: index.TaskStateOpen,
			},
		},
	}

	// Save the index
	indexPath := filepath.Join(repoRoot, "_governator/task-index.json")
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	var stdout, stderr bytes.Buffer
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	result, err := Run(repoRoot, opts)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	// Check that no tasks were resumed or blocked
	if len(result.ResumedTasks) != 0 {
		t.Fatalf("expected 0 resumed tasks, got %d", len(result.ResumedTasks))
	}
	if len(result.BlockedTasks) != 0 {
		t.Fatalf("expected 0 blocked tasks, got %d", len(result.BlockedTasks))
	}

	// Check the result message - should indicate work activity
	if !strings.Contains(result.Message, "work task(s)") {
		t.Fatalf("result message = %q, want to contain 'work task(s)'", result.Message)
	}
}

// TestRunPlanningDriftMessage ensures run outputs a planning drift notice before failing.
func TestRunPlanningDriftMessage(t *testing.T) {
	t.Parallel()

	repoRoot := setupTestRepoWithConfig(t)

	docDir := filepath.Join(repoRoot, "_governator", "docs")
	if err := os.MkdirAll(docDir, 0o755); err != nil {
		t.Fatalf("create docs dir: %v", err)
	}

	planFile := filepath.Join(docDir, "roadmap.md")
	if err := os.WriteFile(planFile, []byte("initial plan"), 0o644); err != nil {
		t.Fatalf("write plan file: %v", err)
	}

	stored, err := digests.Compute(repoRoot)
	if err != nil {
		t.Fatalf("compute digests: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Digests:       stored,
	}

	indexPath := filepath.Join(repoRoot, "_governator/task-index.json")
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	if err := os.WriteFile(planFile, []byte("updated plan"), 0o644); err != nil {
		t.Fatalf("update plan file: %v", err)
	}

	var stdout, stderr bytes.Buffer
	opts := Options{
		Stdout: &stdout,
		Stderr: &stderr,
	}

	_, err = Run(repoRoot, opts)
	if err == nil {
		t.Fatal("expected planning drift error")
	}
	if !errors.Is(err, ErrPlanningDrift) {
		t.Fatalf("error = %v, want ErrPlanningDrift", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "planning=drift status=blocked") {
		t.Fatalf("stdout = %q, want planning drift prefix", output)
	}
	if !strings.Contains(output, "governator plan") {
		t.Fatalf("stdout = %q, want plan guidance", output)
	}
}

// TestRunValidation ensures proper validation of inputs.
func TestRunValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		repoRoot string
		opts     Options
		wantErr  string
	}{
		{
			name:     "empty repo root",
			repoRoot: "",
			opts:     Options{Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}},
			wantErr:  "repo root is required",
		},
		{
			name:     "nil stdout",
			repoRoot: "/tmp",
			opts:     Options{Stdout: nil, Stderr: &bytes.Buffer{}},
			wantErr:  "stdout and stderr are required",
		},
		{
			name:     "nil stderr",
			repoRoot: "/tmp",
			opts:     Options{Stdout: &bytes.Buffer{}, Stderr: nil},
			wantErr:  "stdout and stderr are required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Run(tt.repoRoot, tt.opts)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

// setupTestRepoWithConfig creates a temporary git repository with config for testing.
func setupTestRepoWithConfig(t *testing.T) string {
	t.Helper()
	repoRoot := testrepos.New(t).Root

	// Create config directory and file
	configDir := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("create config directory: %v", err)
	}

	configPath := filepath.Join(configDir, "config.json")
	configContent := `{
		"workers": {
			"commands": {
				"default": ["echo", "processing {task_path}"]
			}
		},
		"concurrency": {
			"global": 1,
			"default_role": 1
		},
		"timeouts": {
			"worker_seconds": 300
		},
		"retries": {
			"max_attempts": 3
		},
		"auto_rerun": {
			"enabled": false,
			"cooldown_seconds": 60
		}
	}`

	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config file: %v", err)
	}

	// Create GOVERNATOR.md file to avoid planning drift
	governatorPath := filepath.Join(repoRoot, "GOVERNATOR.md")
	governatorContent := "# Test Project\n"
	if err := os.WriteFile(governatorPath, []byte(governatorContent), 0o644); err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}

	// Create placeholder role prompts required by phase runner
	rolesDir := filepath.Join(repoRoot, "_governator", "roles")
	if err := os.MkdirAll(rolesDir, 0o755); err != nil {
		t.Fatalf("create roles dir: %v", err)
	}
	for _, role := range []string{"architect", "generalist", "planner"} {
		path := filepath.Join(rolesDir, fmt.Sprintf("%s.md", role))
		content := fmt.Sprintf("# Role: %s\nRole prompt placeholder.\n", role)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write role prompt %s: %v", role, err)
		}
	}

	// Create planning prompts
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("create prompts dir: %v", err)
	}
	promptMap := map[string]string{
		"architecture-baseline.md": "Architecture baseline prompt placeholder.",
		"gap-analysis.md":          "Gap analysis prompt placeholder.",
		"roadmap.md":               "Roadmap prompt placeholder.",
		"task-planning.md":         "Task planning prompt placeholder.",
	}
	for name, content := range promptMap {
		path := filepath.Join(promptsDir, name)
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			t.Fatalf("write prompt %s: %v", name, err)
		}
	}

	workerContract := filepath.Join(repoRoot, "_governator", "worker-contract.md")
	if err := os.WriteFile(workerContract, []byte("# Worker Contract\n\nPlaceholder.\n"), 0o644); err != nil {
		t.Fatalf("write worker contract: %v", err)
	}

	stateStore := phase.NewStore(repoRoot)
	state := phase.DefaultState()
	state.Current = phase.PhaseExecution
	state.LastCompleted = phase.PhaseTaskPlanning
	for _, p := range []phase.Phase{
		phase.PhaseArchitectureBaseline,
		phase.PhaseGapAnalysis,
		phase.PhaseProjectPlanning,
		phase.PhaseTaskPlanning,
	} {
		record := state.RecordFor(p)
		record.CompletedAt = time.Now().UTC()
		state.SetRecord(p, record)
	}
	if err := stateStore.Save(state); err != nil {
		t.Fatalf("save phase state: %v", err)
	}

	return repoRoot
}

// computeTestDigest computes the digest for test GOVERNATOR.md content.
func computeTestDigest() string {
	content := "# Test Project\n"
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", sum)
}

// findTaskByID locates a task in the index for testing.
func findTaskByID(t *testing.T, idx index.Index, taskID string) index.Task {
	t.Helper()
	for _, task := range idx.Tasks {
		if task.ID == taskID {
			return task
		}
	}
	t.Fatalf("task %q not found in index", taskID)
	return index.Task{}
}
