// Package run provides planning supervisor control helpers.
package run

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/supervisor"
)

// PlanningSupervisorStopOptions configures stop behavior for the planning supervisor.
type PlanningSupervisorStopOptions struct {
	StopWorker bool
}

// StopPlanningSupervisor terminates the planning supervisor and optionally its active worker.
func StopPlanningSupervisor(repoRoot string, opts PlanningSupervisorStopOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	state, ok, err := supervisor.LoadPlanningState(repoRoot)
	if err != nil {
		return err
	}
	if !ok || state.PID <= 0 {
		return supervisor.ErrPlanningSupervisorNotRunning
	}
	_, running, err := supervisor.PlanningSupervisorRunning(repoRoot)
	if err != nil {
		return err
	}
	if !running {
		state.State = supervisor.SupervisorStateStopped
		state.Error = ""
		state.LastTransition = time.Now().UTC()
		_ = supervisor.SavePlanningState(repoRoot, state)
		return supervisor.ErrPlanningSupervisorNotRunning
	}

	if opts.StopWorker {
		if err := stopPlanningWorker(repoRoot, state); err != nil {
			return err
		}
	}

	if err := terminateProcess(state.PID); err != nil {
		return err
	}

	state.State = supervisor.SupervisorStateStopped
	state.Error = ""
	state.LastTransition = time.Now().UTC()
	return supervisor.SavePlanningState(repoRoot, state)
}

func stopPlanningWorker(repoRoot string, state supervisor.PlanningSupervisorState) error {
	workerStateDir := strings.TrimSpace(state.WorkerStateDir)
	wrapperPID := state.WorkerPID
	if workerStateDir == "" {
		store, err := inflight.NewStore(repoRoot)
		if err != nil {
			return fmt.Errorf("create in-flight store: %w", err)
		}
		set, err := store.Load()
		if err != nil {
			return fmt.Errorf("load in-flight tasks: %w", err)
		}
		for _, entry := range set {
			if entry.ID == planningIndexTaskID {
				workerStateDir = entry.WorkerStateDir
				break
			}
		}
	}
	if workerStateDir == "" {
		return nil
	}
	killWorkerProcess(wrapperPID, workerStateDir, nil)
	return nil
}

func terminateProcess(pid int) error {
	if pid <= 0 {
		return nil
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find supervisor pid %d: %w", pid, err)
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return fmt.Errorf("terminate supervisor pid %d: %w", pid, err)
	}
	return nil
}
