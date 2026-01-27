// Package run defines the planning workstream controller implementation.
package run

import (
	"fmt"

	"github.com/cmtonkinson/governator/internal/phase"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
)

// planningController adapts phase state to the generic workstream runner.
type planningController struct {
	runner *phaseRunner
	state  *phase.State
}

// newPlanningController constructs a controller for the planning workstream.
func newPlanningController(runner *phaseRunner, state *phase.State) *planningController {
	return &planningController{
		runner: runner,
		state:  state,
	}
}

// CurrentStep returns the active planning step derived from phase state.
func (controller *planningController) CurrentStep() (workstreamStep, bool) {
	if controller.state.Current >= phase.PhaseExecution {
		return workstreamStep{}, false
	}
	step, ok := controller.runner.planning.stepForPhase(controller.state.Current)
	if !ok {
		return workstreamStep{}, false
	}
	return step, true
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

// Advance advances the phase state after a completed step when configured.
func (controller *planningController) Advance(step workstreamStep, collect workstreamCollectResult) (bool, error) {
	if !collect.Completed {
		return false, nil
	}
	before := controller.state.Current
	if err := controller.runner.completePhase(controller.state); err != nil {
		return false, err
	}
	return controller.state.Current != before, nil
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
