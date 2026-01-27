// Package run defines the planning workstream as an ordered compound task.
package run

import (
	"path/filepath"

	"github.com/cmtonkinson/governator/internal/phase"
)

// planningTask provides ordered lookup for planning steps using the shared workstream schema.
type planningTask struct {
	ordered []workstreamStep
	byPhase map[phase.Phase]workstreamStep
}

// newPlanningTask builds the current planning workstream definition.
func newPlanningTask() planningTask {
	ordered := []workstreamStep{
		{
			phase:      phase.PhaseArchitectureBaseline,
			name:       "architecture-baseline",
			promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "architecture-baseline.md")),
			role:       "architect",
			actions: workstreamStepActions{
				mergeToBase:  true,
				advancePhase: true,
			},
			gates: workstreamStepGates{
				beforeDispatch: workstreamGateTarget{enabled: true, phase: phase.PhaseArchitectureBaseline},
				beforeAdvance:  workstreamGateTarget{enabled: true, phase: phase.PhaseArchitectureBaseline.Next()},
			},
		},
		{
			phase:      phase.PhaseGapAnalysis,
			name:       "gap-analysis",
			promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "gap-analysis.md")),
			role:       "default",
			actions: workstreamStepActions{
				mergeToBase:  true,
				advancePhase: true,
			},
			gates: workstreamStepGates{
				beforeDispatch: workstreamGateTarget{enabled: true, phase: phase.PhaseGapAnalysis},
				beforeAdvance:  workstreamGateTarget{enabled: true, phase: phase.PhaseGapAnalysis.Next()},
			},
		},
		{
			phase:      phase.PhaseProjectPlanning,
			name:       "project-planning",
			promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "roadmap.md")),
			role:       "planner",
			actions: workstreamStepActions{
				mergeToBase:  true,
				advancePhase: true,
			},
			gates: workstreamStepGates{
				beforeDispatch: workstreamGateTarget{enabled: true, phase: phase.PhaseProjectPlanning},
				beforeAdvance:  workstreamGateTarget{enabled: true, phase: phase.PhaseProjectPlanning.Next()},
			},
		},
		{
			phase:      phase.PhaseTaskPlanning,
			name:       "task-planning",
			promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "task-planning.md")),
			role:       "planner",
			actions: workstreamStepActions{
				mergeToBase:  true,
				advancePhase: true,
			},
			gates: workstreamStepGates{
				beforeDispatch: workstreamGateTarget{enabled: true, phase: phase.PhaseTaskPlanning},
				beforeAdvance:  workstreamGateTarget{enabled: true, phase: phase.PhaseTaskPlanning.Next()},
			},
		},
	}
	byPhase := make(map[phase.Phase]workstreamStep, len(ordered))
	for _, step := range ordered {
		byPhase[step.phase] = step
	}
	return planningTask{
		ordered: ordered,
		byPhase: byPhase,
	}
}

// stepForPhase returns the planning step that corresponds to the given phase.
func (task planningTask) stepForPhase(p phase.Phase) (workstreamStep, bool) {
	step, ok := task.byPhase[p]
	return step, ok
}
