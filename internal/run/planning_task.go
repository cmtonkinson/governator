// Package run defines the planning workstream as an ordered compound task.
package run

import "github.com/cmtonkinson/governator/internal/phase"

// planningTask provides ordered lookup for planning steps using the shared workstream schema.
type planningTask struct {
	ordered []workstreamStep
	byID    map[string]workstreamStep
}

// newPlanningTask builds the planning workstream definition from the planning spec.
func newPlanningTask(repoRoot string) (planningTask, error) {
	spec, err := LoadPlanningSpec(repoRoot)
	if err != nil {
		return planningTask{}, err
	}
	return planningTaskFromSpec(spec)
}

// stepForID returns the planning step that corresponds to the given step ID.
func (task planningTask) stepForID(id string) (workstreamStep, bool) {
	step, ok := task.byID[id]
	return step, ok
}

// stepForPhase returns the planning step that corresponds to the given phase (legacy compatibility).
func (task planningTask) stepForPhase(p phase.Phase) (workstreamStep, bool) {
	// Map legacy phases to step IDs for backward compatibility
	var stepID string
	switch p {
	case phase.PhaseArchitectureBaseline:
		stepID = "architecture-baseline"
	case phase.PhaseGapAnalysis:
		stepID = "gap-analysis"
	case phase.PhaseProjectPlanning:
		stepID = "project-planning"
	case phase.PhaseTaskPlanning:
		stepID = "task-planning"
	default:
		return workstreamStep{}, false
	}
	return task.stepForID(stepID)
}
