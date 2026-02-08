// Package run provides supervisor control helpers.
package run

import (
	"fmt"
	"strings"
	"time"

	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/supervisor"
)

// UnifiedSupervisorStopOptions configures stop behavior for the unified supervisor.
type UnifiedSupervisorStopOptions struct {
	StopWorker bool
}

// StopUnifiedSupervisor terminates the unified supervisor and optionally its active workers.
func StopUnifiedSupervisor(repoRoot string, opts UnifiedSupervisorStopOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	state, ok, err := supervisor.LoadExecutionState(repoRoot)
	if err != nil {
		return err
	}
	if !ok || state.PID <= 0 {
		return supervisor.ErrSupervisorNotRunning
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
		return supervisor.ErrSupervisorNotRunning
	}

	if opts.StopWorker {
		if err := stopExecutionWorkers(repoRoot); err != nil {
			return err
		}
	}

	if err := TerminateProcess(state.PID); err != nil {
		return err
	}

	state.State = supervisor.SupervisorStateStopped
	state.Error = ""
	state.LastTransition = time.Now().UTC()
	return supervisor.SaveExecutionState(repoRoot, state)
}

// ExecutionSupervisorStopOptions configures stop behavior for the legacy execution supervisor API.
// Deprecated: use UnifiedSupervisorStopOptions.
type ExecutionSupervisorStopOptions = UnifiedSupervisorStopOptions

// StopExecutionSupervisor terminates the unified supervisor.
// Deprecated: use StopUnifiedSupervisor.
func StopExecutionSupervisor(repoRoot string, opts ExecutionSupervisorStopOptions) error {
	return StopUnifiedSupervisor(repoRoot, UnifiedSupervisorStopOptions(opts))
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
