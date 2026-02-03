// Package run provides execution supervisor control helpers.
package run

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/supervisor"
)

// ExecutionSupervisorStopOptions configures stop behavior for the execution supervisor.
type ExecutionSupervisorStopOptions struct {
	StopWorker bool
}

// StopExecutionSupervisor terminates the execution supervisor and optionally its active workers.
func StopExecutionSupervisor(repoRoot string, opts ExecutionSupervisorStopOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	state, ok, err := supervisor.LoadExecutionState(repoRoot)
	if err != nil {
		return err
	}
	if !ok || state.PID <= 0 {
		return supervisor.ErrExecutionSupervisorNotRunning
	}
	_, running, err := supervisor.ExecutionSupervisorRunning(repoRoot)
	if err != nil {
		return err
	}
	if !running {
		state.State = supervisor.SupervisorStateStopped
		state.Error = ""
		state.LastTransition = time.Now().UTC()
		_ = supervisor.SaveExecutionState(repoRoot, state)
		return supervisor.ErrExecutionSupervisorNotRunning
	}

	if opts.StopWorker {
		if err := stopExecutionWorkers(repoRoot); err != nil {
			return err
		}
	}

	if err := terminateProcess(state.PID); err != nil {
		return err
	}

	state.State = supervisor.SupervisorStateStopped
	state.Error = ""
	state.LastTransition = time.Now().UTC()
	return supervisor.SaveExecutionState(repoRoot, state)
}

// stopExecutionWorkers attempts to terminate all in-flight execution workers.
func stopExecutionWorkers(repoRoot string) error {
	store, err := inflight.NewStore(repoRoot)
	if err != nil {
		return fmt.Errorf("create in-flight store: %w", err)
	}
	set, err := store.Load()
	if err != nil {
		return fmt.Errorf("load in-flight tasks: %w", err)
	}
	for _, entry := range set {
		wrapperPID, _ := readDispatchWrapperPID(entry.WorkerStateDir)
		killWorkerProcess(wrapperPID, entry.WorkerStateDir, nil)
	}
	return nil
}
