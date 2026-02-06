package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/supervisor"
)

func TestSummaryString(t *testing.T) {
	empty := Summary{}
	if got := empty.String(); got != "backlog=0 merged=0 in-progress=0" {
		t.Fatalf("empty summary string = %q", got)
	}

	withRows := Summary{
		Backlog:    1,
		Merged:     1,
		InProgress: 1,
		Rows: []statusRow{
			{id: "T-100", state: "triaged", pid: "1234", role: "builder", attrs: "blocked", title: "A task", order: 0},
		},
	}
	result := withRows.String()
	if !strings.Contains(result, "backlog=1 merged=1 in-progress=1") {
		t.Fatalf("summary header missing counts: %q", result)
	}
	if !strings.Contains(result, "id") || !strings.Contains(result, "state") {
		t.Fatalf("table header missing: %q", result)
	}
}

func TestGetSummary(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "governator-status-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	stateDir := filepath.Join(tempDir, "_governator")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	longTitle := strings.Repeat("x", titleMaxWidth+10)
	testIndex := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{ID: "001-backlog-task", Kind: index.TaskKindExecution, State: index.TaskStateBacklog},
			{ID: "002-triaged-task", Kind: index.TaskKindExecution, State: index.TaskStateTriaged, Role: "dev", AssignedRole: "dev"},
			{ID: "003-implemented-task", Kind: index.TaskKindExecution, State: index.TaskStateImplemented, Role: "dev"},
			{ID: "004-tested-task", Kind: index.TaskKindExecution, State: index.TaskStateTested, Role: "dev"},
			{ID: "005-reviewed-task", Kind: index.TaskKindExecution, State: index.TaskStateReviewed, Role: "dev"},
			{ID: "006-mergeable-task", Kind: index.TaskKindExecution, State: index.TaskStateMergeable, Role: "dev"},
			{ID: "007-merged-task", Kind: index.TaskKindExecution, State: index.TaskStateMerged, Role: "dev"},
			{ID: "008-blocked-task", Kind: index.TaskKindExecution, State: index.TaskStateBlocked, Role: "dev", BlockedReason: "blocked"},
			{ID: "009-conflict-task", Kind: index.TaskKindExecution, State: index.TaskStateConflict, Role: "dev", MergeConflict: true},
			{ID: "010-resolved-task", Kind: index.TaskKindExecution, State: index.TaskStateResolved, Role: "dev", Title: longTitle},
		},
	}

	indexPath := filepath.Join(tempDir, "_governator", "_local-state", "index.json")
	if err := index.Save(indexPath, testIndex); err != nil {
		t.Fatalf("failed to save test index: %v", err)
	}

	summary, err := GetSummary(tempDir)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	if summary.Backlog != 1 {
		t.Fatalf("expected 1 backlog task, got %d", summary.Backlog)
	}
	if summary.Merged != 1 {
		t.Fatalf("expected 1 merged task, got %d", summary.Merged)
	}
	if summary.InProgress != 8 {
		t.Fatalf("expected 8 in-progress tasks, got %d", summary.InProgress)
	}
	if len(summary.Rows) != summary.InProgress {
		t.Fatalf("expected %d rows, got %d", summary.InProgress, len(summary.Rows))
	}

	if summary.Rows[0].state != string(index.TaskStateTriaged) {
		t.Fatalf("expected first row state triaged, got %s", summary.Rows[0].state)
	}
	last := summary.Rows[len(summary.Rows)-1]
	if !strings.HasSuffix(last.title, "...") {
		t.Fatalf("expected truncated title, got %q", last.title)
	}

	foundAttrs := false
	for _, row := range summary.Rows {
		if row.id == "008" && row.attrs == "blocked" {
			foundAttrs = true
		}
	}
	if !foundAttrs {
		t.Fatalf("blocked attribute missing from rows: %+v", summary.Rows)
	}
}

// TestGetSummarySupervisorFiltering ensures status only reports running or failed supervisors.
func TestGetSummarySupervisorFiltering(t *testing.T) {
	t.Parallel()

	t.Run("running_included", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		stateDir := filepath.Join(repoRoot, "_governator")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("create state dir: %v", err)
		}
		indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
		if err := index.Save(indexPath, index.Index{SchemaVersion: 1}); err != nil {
			t.Fatalf("save index: %v", err)
		}
		now := time.Now().UTC()
		if err := supervisor.SavePlanningState(repoRoot, supervisor.PlanningSupervisorState{
			Phase:          "planning",
			PID:            os.Getpid(),
			State:          supervisor.SupervisorStateRunning,
			StartedAt:      now,
			LastTransition: now,
			LogPath:        supervisor.PlanningLogPath(repoRoot),
		}); err != nil {
			t.Fatalf("save planning state: %v", err)
		}

		summary, err := GetSummary(repoRoot)
		if err != nil {
			t.Fatalf("GetSummary() failed: %v", err)
		}
		if len(summary.Supervisors) != 1 {
			t.Fatalf("supervisors=%d, want 1", len(summary.Supervisors))
		}
		if summary.Supervisors[0].State != string(supervisor.SupervisorStateRunning) {
			t.Fatalf("state=%s, want %s", summary.Supervisors[0].State, supervisor.SupervisorStateRunning)
		}
		output := summary.String()
		if !strings.Contains(output, "supervisors") {
			t.Fatalf("expected supervisors header in output: %q", output)
		}
		if !strings.Contains(output, "runtime") {
			t.Fatalf("expected runtime column in output: %q", output)
		}
	})

	t.Run("stopped_excluded", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		stateDir := filepath.Join(repoRoot, "_governator")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("create state dir: %v", err)
		}
		indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
		if err := index.Save(indexPath, index.Index{SchemaVersion: 1}); err != nil {
			t.Fatalf("save index: %v", err)
		}
		now := time.Now().UTC()
		if err := supervisor.SaveExecutionState(repoRoot, supervisor.ExecutionSupervisorState{
			Phase:          "execution",
			PID:            os.Getpid(),
			State:          supervisor.SupervisorStateStopped,
			StartedAt:      now,
			LastTransition: now,
			LogPath:        supervisor.ExecutionLogPath(repoRoot),
		}); err != nil {
			t.Fatalf("save execution state: %v", err)
		}

		summary, err := GetSummary(repoRoot)
		if err != nil {
			t.Fatalf("GetSummary() failed: %v", err)
		}
		if len(summary.Supervisors) != 0 {
			t.Fatalf("supervisors=%d, want 0", len(summary.Supervisors))
		}
	})

	t.Run("failed_included", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		stateDir := filepath.Join(repoRoot, "_governator")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("create state dir: %v", err)
		}
		indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
		if err := index.Save(indexPath, index.Index{SchemaVersion: 1}); err != nil {
			t.Fatalf("save index: %v", err)
		}
		now := time.Now().UTC()
		if err := supervisor.SaveExecutionState(repoRoot, supervisor.ExecutionSupervisorState{
			Phase:          "execution",
			PID:            0,
			State:          supervisor.SupervisorStateFailed,
			StartedAt:      now,
			LastTransition: now,
			LogPath:        supervisor.ExecutionLogPath(repoRoot),
		}); err != nil {
			t.Fatalf("save execution state: %v", err)
		}

		summary, err := GetSummary(repoRoot)
		if err != nil {
			t.Fatalf("GetSummary() failed: %v", err)
		}
		if len(summary.Supervisors) != 1 {
			t.Fatalf("supervisors=%d, want 1", len(summary.Supervisors))
		}
		if summary.Supervisors[0].State != string(supervisor.SupervisorStateFailed) {
			t.Fatalf("state=%s, want %s", summary.Supervisors[0].State, supervisor.SupervisorStateFailed)
		}
	})

	t.Run("stale_running_marked_failed", func(t *testing.T) {
		t.Parallel()
		repoRoot := t.TempDir()
		stateDir := filepath.Join(repoRoot, "_governator")
		if err := os.MkdirAll(stateDir, 0o755); err != nil {
			t.Fatalf("create state dir: %v", err)
		}
		indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
		if err := index.Save(indexPath, index.Index{SchemaVersion: 1}); err != nil {
			t.Fatalf("save index: %v", err)
		}
		now := time.Now().UTC()
		if err := supervisor.SaveExecutionState(repoRoot, supervisor.ExecutionSupervisorState{
			Phase:          "execution",
			PID:            999999,
			State:          supervisor.SupervisorStateRunning,
			StartedAt:      now,
			LastTransition: now,
			LogPath:        supervisor.ExecutionLogPath(repoRoot),
		}); err != nil {
			t.Fatalf("save execution state: %v", err)
		}

		summary, err := GetSummary(repoRoot)
		if err != nil {
			t.Fatalf("GetSummary() failed: %v", err)
		}
		if len(summary.Supervisors) != 1 {
			t.Fatalf("supervisors=%d, want 1", len(summary.Supervisors))
		}
		if summary.Supervisors[0].State != string(supervisor.SupervisorStateFailed) {
			t.Fatalf("state=%s, want %s", summary.Supervisors[0].State, supervisor.SupervisorStateFailed)
		}
	})
}

// TestPlanningStepSummary_NotStartedState ensures no planning steps are shown when state is PlanningNotStartedState.
func TestPlanningStepSummary_NotStartedState(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	// Create planning spec
	planningSpecPath := filepath.Join(repoRoot, "_governator", "planning.json")
	planningSpec := `{
		"version": 2,
		"steps": [
			{
				"id": "architecture-baseline",
				"name": "Architecture Baseline",
				"prompt": "_governator/prompts/architecture-baseline.md",
				"role": "architect"
			},
			{
				"id": "gap-analysis",
				"name": "Gap Analysis",
				"prompt": "_governator/prompts/gap-analysis.md",
				"role": "default"
			}
		]
	}`
	if err := os.WriteFile(planningSpecPath, []byte(planningSpec), 0o644); err != nil {
		t.Fatalf("write planning spec: %v", err)
	}

	// Create index with PlanningNotStartedState
	testIndex := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    "planning",
				Kind:  index.TaskKindPlanning,
				State: "governator_planning_not_started",
			},
		},
	}

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	if err := index.Save(indexPath, testIndex); err != nil {
		t.Fatalf("save index: %v", err)
	}

	summary, err := GetSummary(repoRoot)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	// Should NOT show any planning steps when state is PlanningNotStartedState
	if len(summary.PlanningSteps) != 0 {
		t.Fatalf("expected 0 planning steps for PlanningNotStartedState, got %d", len(summary.PlanningSteps))
	}

	output := summary.String()
	if strings.Contains(output, "planning-steps") {
		t.Fatalf("output should not contain planning-steps header: %q", output)
	}
}

// TestPlanningStepSummary_InProgress ensures planning steps are shown when state is an actual step ID.
func TestPlanningStepSummary_InProgress(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	// Create planning spec
	planningSpecPath := filepath.Join(repoRoot, "_governator", "planning.json")
	planningSpec := `{
		"version": 2,
		"steps": [
			{
				"id": "architecture-baseline",
				"name": "Architecture Baseline",
				"prompt": "_governator/prompts/architecture-baseline.md",
				"role": "architect"
			},
			{
				"id": "gap-analysis",
				"name": "Gap Analysis",
				"prompt": "_governator/prompts/gap-analysis.md",
				"role": "default"
			}
		]
	}`
	if err := os.WriteFile(planningSpecPath, []byte(planningSpec), 0o644); err != nil {
		t.Fatalf("write planning spec: %v", err)
	}

	// Create index with gap-analysis as current step (second step)
	testIndex := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    "planning",
				Kind:  index.TaskKindPlanning,
				State: "gap-analysis",
			},
		},
	}

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	if err := index.Save(indexPath, testIndex); err != nil {
		t.Fatalf("save index: %v", err)
	}

	summary, err := GetSummary(repoRoot)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	// Should show planning steps when state is an actual step ID
	if len(summary.PlanningSteps) != 2 {
		t.Fatalf("expected 2 planning steps, got %d", len(summary.PlanningSteps))
	}

	// First step should be complete
	if summary.PlanningSteps[0].ID != "architecture-baseline" {
		t.Fatalf("first step ID = %q, want architecture-baseline", summary.PlanningSteps[0].ID)
	}
	if summary.PlanningSteps[0].Status != "complete" {
		t.Fatalf("first step status = %q, want complete", summary.PlanningSteps[0].Status)
	}

	// Second step should be in-progress
	if summary.PlanningSteps[1].ID != "gap-analysis" {
		t.Fatalf("second step ID = %q, want gap-analysis", summary.PlanningSteps[1].ID)
	}
	if summary.PlanningSteps[1].Status != "in-progress" {
		t.Fatalf("second step status = %q, want in-progress", summary.PlanningSteps[1].Status)
	}

	output := summary.String()
	if !strings.Contains(output, "planning-steps=2") {
		t.Fatalf("output should contain planning-steps=2: %q", output)
	}
}

// TestPlanningStepSummary_Complete ensures no planning steps are shown when planning is complete.
func TestPlanningStepSummary_Complete(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()
	stateDir := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatalf("create state dir: %v", err)
	}

	// Create planning spec
	planningSpecPath := filepath.Join(repoRoot, "_governator", "planning.json")
	planningSpec := `{
		"version": 2,
		"steps": [
			{
				"id": "architecture-baseline",
				"name": "Architecture Baseline",
				"prompt": "_governator/prompts/architecture-baseline.md",
				"role": "architect"
			}
		]
	}`
	if err := os.WriteFile(planningSpecPath, []byte(planningSpec), 0o644); err != nil {
		t.Fatalf("write planning spec: %v", err)
	}

	// Create index with PlanningCompleteState
	testIndex := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    "planning",
				Kind:  index.TaskKindPlanning,
				State: "governator_planning_complete",
			},
		},
	}

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	if err := index.Save(indexPath, testIndex); err != nil {
		t.Fatalf("save index: %v", err)
	}

	summary, err := GetSummary(repoRoot)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	// Should NOT show any planning steps when planning is complete
	if len(summary.PlanningSteps) != 0 {
		t.Fatalf("expected 0 planning steps for PlanningCompleteState, got %d", len(summary.PlanningSteps))
	}

	output := summary.String()
	if strings.Contains(output, "planning-steps") {
		t.Fatalf("output should not contain planning-steps header: %q", output)
	}
}
