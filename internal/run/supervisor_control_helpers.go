package run

import (
	"errors"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/supervisor"
)

// supervisorStateEqual compares two supervisor state snapshots for equality.
func SupervisorStateEqual(left supervisor.SupervisorStateInfo, right supervisor.SupervisorStateInfo) bool {
	return left.Phase == right.Phase &&
		left.PID == right.PID &&
		left.WorkerPID == right.WorkerPID &&
		left.ValidationPID == right.ValidationPID &&
		left.StepID == right.StepID &&
		left.StepName == right.StepName &&
		left.State == right.State &&
		left.LogPath == right.LogPath &&
		left.Error == right.Error &&
		left.WorkerStateDir == right.WorkerStateDir
}

// MarkSupervisorTransition updates the transition timestamp for the supervisor state.
func MarkSupervisorTransition(state supervisor.SupervisorStateInfo) supervisor.SupervisorStateInfo {
	state.LastTransition = time.Now().UTC()
	return state
}

// TerminateProcess attempts to terminate a process by PID.
func TerminateProcess(pid int) error {
	if pid <= 0 {
		return nil // No process to terminate
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("find process %d: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return fmt.Errorf("terminate process %d: %w", pid, err)
	}
	return nil
}
