// Tests for the planning compound task definition.
package run

import (
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/phase"
)

func TestPlanningTaskStepForPhase(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)
	task, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("load planning spec: %v", err)
	}
	step, ok := task.stepForPhase(phase.PhaseArchitectureBaseline)
	if !ok {
		t.Fatalf("expected architecture step")
	}
	if step.workstreamID() != planningIndexTaskID {
		t.Fatalf("workstream id = %q", step.workstreamID())
	}
	expectedPrompt := filepath.ToSlash(filepath.Join("_governator", "prompts", "architecture-baseline.md"))
	if step.promptPath != expectedPrompt {
		t.Fatalf("prompt path = %q, want %q", step.promptPath, expectedPrompt)
	}
	if !step.actions.mergeToBase || !step.actions.advancePhase {
		t.Fatalf("expected deterministic success actions to be enabled")
	}
	// New structure uses nextStepID instead of gates
	if step.nextStepID != "gap-analysis" {
		t.Fatalf("unexpected next step ID: %q, want %q", step.nextStepID, "gap-analysis")
	}
}

func TestPlanningTaskStepForPhaseMissing(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeTestPlanningSpec(t, repoRoot)
	task, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("load planning spec: %v", err)
	}
	if _, ok := task.stepForPhase(phase.PhaseExecution); ok {
		t.Fatalf("execution phase should not be part of planning compound task")
	}
}
