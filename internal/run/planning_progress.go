// Package run provides helpers for determining planning progress from the task index.
package run

import (
	"fmt"

	"github.com/cmtonkinson/governator/internal/index"
)

// currentPlanningStep returns the first planning step not yet marked merged.
func currentPlanningStep(idx index.Index, planning planningTask) (workstreamStep, bool, error) {
	for _, step := range planning.ordered {
		taskID := planningTaskID(step)
		task, err := findIndexTask(&idx, taskID)
		if err != nil {
			return workstreamStep{}, false, err
		}
		if task.Kind != index.TaskKindPlanning {
			return workstreamStep{}, false, fmt.Errorf("planning task %s has unexpected kind %q", taskID, task.Kind)
		}
		if task.State != index.TaskStateMerged {
			return step, true, nil
		}
	}
	return workstreamStep{}, false, nil
}

// planningComplete reports whether every planning task is merged.
func planningComplete(idx index.Index, planning planningTask) (bool, error) {
	_, ok, err := currentPlanningStep(idx, planning)
	if err != nil {
		return false, err
	}
	return !ok, nil
}
