// Package run defines the planning workstream controller implementation.
package run

import (
	"fmt"
	"path/filepath"

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

// Advance finalizes a completed planning step and refreshes the task index.
func (controller *planningController) Advance(step workstreamStep, collect workstreamCollectResult) (bool, error) {
	if !collect.Completed {
		return false, nil
	}
	if err := controller.runner.completePhase(step); err != nil {
		return false, err
	}
	if err := controller.reloadIndex(); err != nil {
		return false, err
	}
	return true, nil
}

// GateBeforeDispatch checks the configured gate before dispatching the step.
func (controller *planningController) GateBeforeDispatch(step workstreamStep) error {
	return controller.runner.ensureStepGate(step.gates.beforeDispatch, step.phase)
}

// Dispatch starts the worker for the planning step.
func (controller *planningController) Dispatch(step workstreamStep) (workstreamDispatchResult, error) {
	if err := controller.runner.dispatchPhase(step); err != nil {
		return workstreamDispatchResult{}, err
	}
	return workstreamDispatchResult{Handled: true}, nil
}

// EmitRunning logs that the phase worker is still running.
func (controller *planningController) EmitRunning(step workstreamStep, pids []int) {
	if len(pids) == 0 {
		return
	}
	controller.runner.emitPhaseRunning(step.phase, pids[0])
}

// EmitAgentComplete logs that the phase worker has exited.
func (controller *planningController) EmitAgentComplete(step workstreamStep, collect workstreamCollectResult) {
	controller.runner.emitPhaseAgentComplete(step.phase, collect.CompletedPID)
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
