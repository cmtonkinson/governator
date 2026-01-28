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
	if step.workstreamID() != "planning-architecture-baseline" {
		t.Fatalf("workstream id = %q", step.workstreamID())
	}
	expectedPrompt := filepath.ToSlash(filepath.Join("_governator", "prompts", "architecture-baseline.md"))
	if step.promptPath != expectedPrompt {
		t.Fatalf("prompt path = %q, want %q", step.promptPath, expectedPrompt)
	}
	if !step.actions.mergeToBase || !step.actions.advancePhase {
		t.Fatalf("expected deterministic success actions to be enabled")
	}
	if !step.gates.beforeDispatch.enabled || step.gates.beforeDispatch.phase != phase.PhaseArchitectureBaseline {
		t.Fatalf("unexpected dispatch gate: %+v", step.gates.beforeDispatch)
	}
	if !step.gates.beforeAdvance.enabled || step.gates.beforeAdvance.phase != phase.PhaseGapAnalysis {
		t.Fatalf("unexpected advance gate: %+v", step.gates.beforeAdvance)
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
