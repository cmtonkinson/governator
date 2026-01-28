// Package run defines the planning workstream as an ordered compound task.
package run

import "github.com/cmtonkinson/governator/internal/phase"

// planningTask provides ordered lookup for planning steps using the shared workstream schema.
type planningTask struct {
	ordered []workstreamStep
	byPhase map[phase.Phase]workstreamStep
}

// newPlanningTask builds the planning workstream definition from the planning spec.
func newPlanningTask(repoRoot string) (planningTask, error) {
	spec, err := LoadPlanningSpec(repoRoot)
	if err != nil {
		return planningTask{}, err
	}
	return planningTaskFromSpec(spec)
}

// stepForPhase returns the planning step that corresponds to the given phase.
func (task planningTask) stepForPhase(p phase.Phase) (workstreamStep, bool) {
	step, ok := task.byPhase[p]
	return step, ok
}
