// Package run defines the planning workstream controller implementation.
package run

import (
	"fmt"
	"path/filepath"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"

	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

// planningController adapts planning progress to the generic workstream runner.
type planningController struct {
	runner *phaseRunner
	idx    *index.Index
}

// newPlanningController constructs a controller for the planning workstream.
func newPlanningController(runner *phaseRunner, idx *index.Index) *planningController {
	return &planningController{
		runner: runner,
		idx:    idx,
	}
}

// CurrentStep returns the active planning step derived from the task index.
func (controller *planningController) CurrentStep() (workstreamStep, bool, error) {
	if controller.idx == nil {
		return workstreamStep{}, false, fmt.Errorf("task index is required")
	}
	step, ok, err := currentPlanningStep(*controller.idx, controller.runner.planning)
	if err != nil {
		return workstreamStep{}, false, err
	}
	return step, ok, nil
}

// Collect finalizes the planning step when the worker is no longer running.
func (controller *planningController) Collect(step workstreamStep) (workstreamCollectResult, error) {
	result := workstreamCollectResult{}
	taskID := step.workstreamID()
	inFlight := controller.runner.inFlight.Contains(taskID)
	if !inFlight {
		skip, err := controller.shouldSkipStep(step)
		if err != nil {
			return result, err
		}
		if skip {
			result.Completed = true
			result.Handled = true
		}
		return result, nil
	}

	entry, _ := controller.runner.inFlight.Entry(taskID)
	worktreePath, workerStateDir, err := controller.runner.resolvePlanningPaths(step, entry)
	if err != nil {
		return result, err
	}

	runningPID := controller.runner.runningPlanningPID(workerStateDir, taskID, roles.StageWork)
	if runningPID != 0 {
		result.RunningPIDs = []int{runningPID}
		return result, nil
	}

	if err := controller.runner.collectPhaseCompletion(step, worktreePath, workerStateDir); err != nil {
		return result, err
	}

	if pid, found, err := worker.ReadAgentPID(workerStateDir); err == nil && found {
		result.CompletedPID = pid
	}

	if controller.runner.inFlight.Contains(taskID) {
		if err := controller.runner.inFlight.Remove(taskID); err != nil {
			return result, fmt.Errorf("clear planning in-flight: %w", err)
		}
		if err := controller.runner.persistInFlight(); err != nil {
			return result, err
		}
	}

	result.Completed = true
	result.Handled = true
	return result, nil
}

// Advance finalizes a completed planning step, runs validations, and refreshes the task index.
func (controller *planningController) Advance(step workstreamStep, collect workstreamCollectResult) (bool, error) {
	if !collect.Completed {
		return false, nil
	}

	// Run validations after worker completion
	if len(step.validations) > 0 {
		validationEngine := NewValidationEngine(controller.runner.repoRoot)
		results, err := validationEngine.RunValidations(step.name, step.displayName, step.validations)
		if err != nil {
			return false, fmt.Errorf("validation execution failed: %w", err)
		}

		// Check if all validations passed
		allValid := true
		for _, result := range results {
			if !result.Valid {
				allValid = false
				controller.runner.logf("Validation failed for step %s: %s (%s)", step.name, result.Message, result.Type)
				break
			}
		}

		if !allValid {
			return false, fmt.Errorf("validation failed for step %s: planning cannot advance", step.name)
		}
	}

	// Complete the phase and update planning state
	if err := controller.runner.completePhase(step); err != nil {
		return false, err
	}

	// Check if this is the final planning step (transition to execution)
	if step.nextStepID == "" || step.nextStepID == "execution" {
		// Planning is complete - perform task inventory
		taskInventory := NewTaskInventory(controller.runner.repoRoot, controller.idx)
		inventoryResult, err := taskInventory.InventoryTasks()
		if err != nil {
			return false, fmt.Errorf("task inventory failed: %w", err)
		}

		if inventoryResult.TasksAdded == 0 {
			return false, fmt.Errorf("planning completion requires at least one task file in _governator/tasks")
		}

		indexPath := filepath.Join(controller.runner.repoRoot, indexFilePath)
		if err := index.Save(indexPath, *controller.idx); err != nil {
			return false, fmt.Errorf("save task index: %w", err)
		}

		controller.runner.logf("Task inventory completed: added %d tasks to execution backlog", inventoryResult.TasksAdded)

		// Clear planning state to indicate completion
		if err := controller.persistPlanningState(""); err != nil {
			return false, err
		}
	} else {
		// Move to next planning step
		if err := controller.persistPlanningState(step.nextStepID); err != nil {
			return false, err
		}
	}

	return true, nil
}

// GateBeforeDispatch checks the configured gate before dispatching the step.
func (controller *planningController) GateBeforeDispatch(step workstreamStep) error {
	// New validation engine doesn't use phase-based gating
	// Validations are run after worker completion instead
	return nil
}

// Dispatch starts the worker for the planning step.
func (controller *planningController) Dispatch(step workstreamStep) (workstreamDispatchResult, error) {
	if err := controller.runner.dispatchPhase(step); err != nil {
		return workstreamDispatchResult{}, err
	}
	return workstreamDispatchResult{Handled: true}, nil
}

// shouldSkipStep reports whether a planning step can be advanced without dispatching a worker.
func (controller *planningController) shouldSkipStep(step workstreamStep) (bool, error) {
	if controller.idx == nil || !hasExecutionTasks(*controller.idx) {
		return false, nil
	}
	if len(step.validations) == 0 {
		return false, nil
	}
	validationEngine := NewValidationEngine(controller.runner.repoRoot)
	results, err := validationEngine.RunValidations(step.name, step.displayName, step.validations)
	if err != nil {
		return false, fmt.Errorf("validation execution failed: %w", err)
	}
	for _, result := range results {
		if !result.Valid {
			return false, nil
		}
	}
	return true, nil
}

// hasExecutionTasks reports whether the index already contains execution work from a prior plan.
func hasExecutionTasks(idx index.Index) bool {
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindPlanning {
			return true
		}
	}
	return false
}

// EmitRunning logs that the phase worker is still running.
func (controller *planningController) EmitRunning(step workstreamStep, pids []int) {
	if len(pids) == 0 {
		return
	}
	phaseName := stepToPhase(step.name)
	controller.runner.emitPhaseRunning(phaseName, pids[0])
}

// EmitAgentComplete logs that the phase worker has exited.
func (controller *planningController) EmitAgentComplete(step workstreamStep, collect workstreamCollectResult) {
	phaseName := stepToPhase(step.name)
	controller.runner.emitPhaseAgentComplete(phaseName, collect.CompletedPID)
}

// reloadIndex refreshes the planning index state from disk.
func (controller *planningController) reloadIndex() error {
	if controller.idx == nil {
		return fmt.Errorf("task index is required")
	}
	indexPath := filepath.Join(controller.runner.repoRoot, indexFilePath)
	updated, err := index.Load(indexPath)
	if err != nil {
		return fmt.Errorf("reload task index: %w", err)
	}
	*controller.idx = updated
	return nil
}

// persistPlanningState updates the planning state and refreshes planning digests in the task index.
func (controller *planningController) persistPlanningState(nextStepID string) error {
	if controller.idx == nil {
		return fmt.Errorf("task index is required")
	}
	indexPath := filepath.Join(controller.runner.repoRoot, indexFilePath)
	updated, err := index.Load(indexPath)
	if err != nil {
		return fmt.Errorf("reload task index: %w", err)
	}
	digestsMap, err := digests.Compute(controller.runner.repoRoot)
	if err != nil {
		return fmt.Errorf("compute digests: %w", err)
	}
	updated.Digests = digestsMap
	updatePlanningState(&updated, nextStepID)
	if err := index.Save(indexPath, updated); err != nil {
		return fmt.Errorf("save task index: %w", err)
	}
	*controller.idx = updated
	return nil
}
