// Package run provides helpers for terminating timed-out worker processes.
package run

import (
	"fmt"
	"os"
	"syscall"
)

// killWorkerProcess sends SIGKILL to a worker PID when available.
func killWorkerProcess(pid int, warn func(string)) {
	if pid <= 0 {
		return
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		emitKillWarning(warn, fmt.Sprintf("failed to find worker pid %d: %v", pid, err))
		return
	}
	if err := proc.Signal(syscall.SIGKILL); err != nil {
		emitKillWarning(warn, fmt.Sprintf("failed to kill worker pid %d: %v", pid, err))
	}
}

func emitKillWarning(warn func(string), message string) {
	if warn == nil || message == "" {
		return
	}
	warn(message)
}

