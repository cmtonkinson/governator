// Package run provides workstream execution orchestration.
package run

import (
	"errors"

	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/phase"
)

// workstreamController defines the adapter hooks required by the workstream runner.
type workstreamController interface {
	CurrentStep() (workstreamStep, bool)
	StepRecord(step workstreamStep) phase.PhaseRecord
	InFlightEntry(step workstreamStep) (inflight.Entry, bool)
	ResolvePaths(step workstreamStep, entry inflight.Entry) (string, string, error)
	RunningPID(step workstreamStep, record phase.PhaseRecord, workerStateDir string) int
	Collect(step workstreamStep, worktreePath string, workerStateDir string) error
	ClearInFlight(step workstreamStep) error
	MarkFinished(step workstreamStep, record phase.PhaseRecord) error
	Advance(step workstreamStep) (bool, error)
	GateBeforeDispatch(step workstreamStep) error
	Dispatch(step workstreamStep) error
	EmitRunning(step workstreamStep, pid int)
	EmitAgentComplete(step workstreamStep, pid int)
}

// workstreamRunner executes a single workstream using a controller adapter.
type workstreamRunner struct{}

// newWorkstreamRunner returns a ready-to-use workstream runner.
func newWorkstreamRunner() *workstreamRunner {
	return &workstreamRunner{}
}

// Run evaluates the current step, dispatches work, or collects completion.
func (runner *workstreamRunner) Run(controller workstreamController) (bool, error) {
	if controller == nil {
		return false, errors.New("workstream controller is required")
	}

	handled := false
	for {
		step, ok := controller.CurrentStep()
		if !ok {
			return handled, nil
		}
		record := controller.StepRecord(step)
		entry, hasEntry := controller.InFlightEntry(step)

		if hasEntry || (record.Agent.PID != 0 && record.Agent.FinishedAt.IsZero()) {
			worktreePath, workerStateDir, err := controller.ResolvePaths(step, entry)
			if err != nil {
				return handled, err
			}
			if runningPID := controller.RunningPID(step, record, workerStateDir); runningPID != 0 {
				controller.EmitRunning(step, runningPID)
				return true, nil
			}
			if err := controller.Collect(step, worktreePath, workerStateDir); err != nil {
				return handled, err
			}
			if hasEntry {
				if err := controller.ClearInFlight(step); err != nil {
					return handled, err
				}
			}
			if err := controller.MarkFinished(step, record); err != nil {
				return handled, err
			}
			record = controller.StepRecord(step)
			controller.EmitAgentComplete(step, record.Agent.PID)
			handled = true
		}

		if record.Agent.PID != 0 && !record.Agent.FinishedAt.IsZero() {
			advanced, err := controller.Advance(step)
			if err != nil {
				return handled, err
			}
			if advanced {
				handled = true
				continue
			}
		}

		if err := controller.GateBeforeDispatch(step); err != nil {
			return handled, err
		}
		if err := controller.Dispatch(step); err != nil {
			return handled, err
		}
		return true, nil
	}
}
