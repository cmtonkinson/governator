// Package run provides helpers for determining planning progress from the task index.
package run

import (
	"fmt"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

// PlanningCompleteState marks the planning task as completed in the task index.
const PlanningCompleteState = "governator_planning_complete"

// PlanningNotStartedState marks the planning task as not yet started in the task index.
const PlanningNotStartedState = "governator_planning_not_started"

// currentPlanningStep returns the current planning step based on the planning state.
func currentPlanningStep(idx index.Index, planning planningTask) (workstreamStep, bool, error) {
	stateID, err := planningTaskState(idx)
	if err != nil {
		return workstreamStep{}, false, err
	}
	if stateID == "" || stateID == PlanningCompleteState {
		return workstreamStep{}, false, nil
	}
	if stateID == PlanningNotStartedState {
		// Planning hasn't started yet - return the first step
		if len(planning.ordered) == 0 {
			return workstreamStep{}, false, fmt.Errorf("planning spec requires at least one step")
		}
		return planning.ordered[0], true, nil
	}
	if step, ok := planning.stepForID(stateID); ok {
		return step, true, nil
	}
	return workstreamStep{}, false, fmt.Errorf("planning state id %q not found in spec", stateID)
}

// planningComplete reports whether planning is complete (all steps done or state ID empty).
func planningComplete(idx index.Index, planning planningTask) (bool, error) {
	stateID, err := planningTaskState(idx)
	if err != nil {
		return false, err
	}
	return stateID == PlanningCompleteState, nil
}

// updatePlanningState updates the planning state in the index.
func updatePlanningState(idx *index.Index, nextStepID string) {
	if idx == nil {
		return
	}
	updatePlanningTaskState(idx, strings.TrimSpace(nextStepID))
}

func updatePlanningTaskState(idx *index.Index, stateID string) {
	if idx == nil {
		return
	}
	for i := range idx.Tasks {
		if idx.Tasks[i].ID != planningIndexTaskID || idx.Tasks[i].Kind != index.TaskKindPlanning {
			continue
		}
		stateID = strings.TrimSpace(stateID)
		if stateID == "" {
			stateID = PlanningCompleteState
		}
		idx.Tasks[i].State = index.TaskState(stateID)
		idx.Tasks[i].PID = 0
		return
	}
}

func planningTaskState(idx index.Index) (string, error) {
	task, err := findIndexTask(&idx, planningIndexTaskID)
	if err != nil {
		return "", err
	}
	if task.Kind != index.TaskKindPlanning {
		return "", fmt.Errorf("planning task %s has unexpected kind %q", planningIndexTaskID, task.Kind)
	}
	return strings.TrimSpace(string(task.State)), nil
}
