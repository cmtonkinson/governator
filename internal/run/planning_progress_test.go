package run

import (
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

// TestCurrentPlanningStep_NotStartedState ensures currentPlanningStep returns the first step
// when the planning state is PlanningNotStartedState.
func TestCurrentPlanningStep_NotStartedState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)

	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("newPlanningTask failed: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    planningIndexTaskID,
				Kind:  index.TaskKindPlanning,
				State: index.TaskState(PlanningNotStartedState),
			},
		},
	}

	step, ok, err := currentPlanningStep(idx, planning)
	if err != nil {
		t.Fatalf("currentPlanningStep failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected step to be returned when state is PlanningNotStartedState")
	}

	// Should return the first step
	if step.name != planning.ordered[0].name {
		t.Fatalf("step name = %q, want %q", step.name, planning.ordered[0].name)
	}
}

// TestCurrentPlanningStep_CompleteState ensures currentPlanningStep returns no step
// when planning is complete.
func TestCurrentPlanningStep_CompleteState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)

	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("newPlanningTask failed: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    planningIndexTaskID,
				Kind:  index.TaskKindPlanning,
				State: index.TaskState(PlanningCompleteState),
			},
		},
	}

	_, ok, err := currentPlanningStep(idx, planning)
	if err != nil {
		t.Fatalf("currentPlanningStep failed: %v", err)
	}
	if ok {
		t.Fatalf("expected no step to be returned when state is PlanningCompleteState")
	}
}

// TestCurrentPlanningStep_InProgressState ensures currentPlanningStep returns the correct step
// when planning is in progress on a specific step.
func TestCurrentPlanningStep_InProgressState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)

	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("newPlanningTask failed: %v", err)
	}

	// Set state to the second step ID
	secondStepID := planning.ordered[1].name

	idx := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    planningIndexTaskID,
				Kind:  index.TaskKindPlanning,
				State: index.TaskState(secondStepID),
			},
		},
	}

	step, ok, err := currentPlanningStep(idx, planning)
	if err != nil {
		t.Fatalf("currentPlanningStep failed: %v", err)
	}
	if !ok {
		t.Fatalf("expected step to be returned when state is a valid step ID")
	}

	if step.name != secondStepID {
		t.Fatalf("step name = %q, want %q", step.name, secondStepID)
	}
}

// TestCurrentPlanningStep_InvalidState ensures currentPlanningStep returns an error
// when the state contains an invalid step ID.
func TestCurrentPlanningStep_InvalidState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)

	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("newPlanningTask failed: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    planningIndexTaskID,
				Kind:  index.TaskKindPlanning,
				State: "invalid-step-id",
			},
		},
	}

	_, _, err = currentPlanningStep(idx, planning)
	if err == nil {
		t.Fatalf("expected error for invalid step ID, got nil")
	}
}

// TestPlanningComplete_NotStartedState ensures planningComplete returns false
// when state is PlanningNotStartedState.
func TestPlanningComplete_NotStartedState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)

	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("newPlanningTask failed: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    planningIndexTaskID,
				Kind:  index.TaskKindPlanning,
				State: index.TaskState(PlanningNotStartedState),
			},
		},
	}

	complete, err := planningComplete(idx, planning)
	if err != nil {
		t.Fatalf("planningComplete failed: %v", err)
	}
	if complete {
		t.Fatalf("expected planning to be incomplete when state is PlanningNotStartedState")
	}
}

// TestPlanningComplete_CompleteState ensures planningComplete returns true
// when state is PlanningCompleteState.
func TestPlanningComplete_CompleteState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)

	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("newPlanningTask failed: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{
				ID:    planningIndexTaskID,
				Kind:  index.TaskKindPlanning,
				State: index.TaskState(PlanningCompleteState),
			},
		},
	}

	complete, err := planningComplete(idx, planning)
	if err != nil {
		t.Fatalf("planningComplete failed: %v", err)
	}
	if !complete {
		t.Fatalf("expected planning to be complete when state is PlanningCompleteState")
	}
}
