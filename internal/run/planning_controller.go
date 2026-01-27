// Package run defines the planning workstream controller implementation.
package run

import (
	"fmt"

	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/phase"
	"github.com/cmtonkinson/governator/internal/roles"
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

// StepRecord returns the phase record for the step phase.
func (controller *planningController) StepRecord(step workstreamStep) phase.PhaseRecord {
	return controller.state.RecordFor(step.phase)
}

// InFlightEntry returns the in-flight entry for the planning step when present.
func (controller *planningController) InFlightEntry(step workstreamStep) (inflight.Entry, bool) {
	return controller.runner.inFlight.Entry(step.workstreamID())
}

// ResolvePaths resolves the worktree and worker state paths for the step.
func (controller *planningController) ResolvePaths(step workstreamStep, entry inflight.Entry) (string, string, error) {
	return controller.runner.resolvePlanningPaths(step, entry)
}

// RunningPID returns a live pid when available for the step.
func (controller *planningController) RunningPID(step workstreamStep, record phase.PhaseRecord, workerStateDir string) int {
	return controller.runner.runningPlanningPID(record, workerStateDir, step.workstreamID(), roles.StageWork)
}

// Collect finalizes the planning worktree after exit.json is present.
func (controller *planningController) Collect(step workstreamStep, worktreePath string, workerStateDir string) error {
	return controller.runner.collectPhaseCompletion(step, worktreePath, workerStateDir)
}

// ClearInFlight removes the planning step from the in-flight store.
func (controller *planningController) ClearInFlight(step workstreamStep) error {
	if err := controller.runner.inFlight.Remove(step.workstreamID()); err != nil {
		return fmt.Errorf("clear planning in-flight: %w", err)
	}
	return controller.runner.persistInFlight()
}

// MarkFinished updates the phase record after collection.
func (controller *planningController) MarkFinished(step workstreamStep, record phase.PhaseRecord) error {
	record.Agent.FinishedAt = controller.runner.now()
	controller.state.SetRecord(step.phase, record)
	if err := controller.runner.store.Save(*controller.state); err != nil {
		return fmt.Errorf("save phase state: %w", err)
	}
	return nil
}

// Advance advances the phase state after a completed step when configured.
func (controller *planningController) Advance(step workstreamStep) (bool, error) {
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
func (controller *planningController) Dispatch(step workstreamStep) error {
	return controller.runner.dispatchPhase(controller.state, step)
}

// EmitRunning logs that the phase worker is still running.
func (controller *planningController) EmitRunning(step workstreamStep, pid int) {
	controller.runner.emitPhaseRunning(step.phase, pid)
}

// EmitAgentComplete logs that the phase worker has exited.
func (controller *planningController) EmitAgentComplete(step workstreamStep, pid int) {
	controller.runner.emitPhaseAgentComplete(step.phase, pid)
}
