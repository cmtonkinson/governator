// Package run provides workstream execution orchestration.
package run

import "errors"

// workstreamController defines the adapter hooks required by the workstream runner.
type workstreamController interface {
	CurrentStep() (workstreamStep, bool, error)
	Collect(step workstreamStep) (workstreamCollectResult, error)
	Advance(step workstreamStep, collect workstreamCollectResult) (bool, error)
	GateBeforeDispatch(step workstreamStep) error
	Dispatch(step workstreamStep) (workstreamDispatchResult, error)
	EmitRunning(step workstreamStep, pids []int)
	EmitAgentComplete(step workstreamStep, collect workstreamCollectResult)
}

// workstreamCollectResult captures the outcome of a collect pass for a step.
type workstreamCollectResult struct {
	RunningPIDs []int
	CompletedPID int
	Completed   bool
	Handled     bool
}

// workstreamDispatchResult captures the outcome of a dispatch pass for a step.
type workstreamDispatchResult struct {
	Handled  bool
	Continue bool
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
		step, ok, err := controller.CurrentStep()
		if err != nil {
			return handled, err
		}
		if !ok {
			return handled, nil
		}

		collectResult, err := controller.Collect(step)
		if err != nil {
			return handled, err
		}
		if collectResult.Handled {
			handled = true
		}
		if len(collectResult.RunningPIDs) > 0 {
			controller.EmitRunning(step, collectResult.RunningPIDs)
			return true, nil
		}
		if collectResult.Completed {
			controller.EmitAgentComplete(step, collectResult)
		}

		advanced, err := controller.Advance(step, collectResult)
		if err != nil {
			return handled, err
		}
		if advanced {
			handled = true
			continue
		}

		if err := controller.GateBeforeDispatch(step); err != nil {
			return handled, err
		}
		dispatchResult, err := controller.Dispatch(step)
		if err != nil {
			return handled, err
		}
		if dispatchResult.Handled {
			handled = true
		}
		if dispatchResult.Continue {
			continue
		}
		return handled, nil
	}
}
