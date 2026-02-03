// Package run provides shared helpers for supervisor control operations.
package run

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// terminateProcess sends SIGTERM to the provided PID when valid.
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
