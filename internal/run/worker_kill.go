// Package run provides helpers for terminating timed-out worker processes.
package run

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/worker"
)

// killWorkerProcess sends SIGKILL to the agent pid when available, falling back to the wrapper pid.
func killWorkerProcess(wrapperPID int, workerStateDir string, warn func(string)) {
	if pid, found := resolveAgentPID(workerStateDir, warn); found {
		// Let the wrapper observe the child exit and write exit.json.
		killPID(pid, warn)
		return
	}
	if strings.TrimSpace(workerStateDir) != "" {
		emitKillWarning(warn, fmt.Sprintf("agent pidfile missing; killing wrapper pid %d", wrapperPID))
	}
	killPID(wrapperPID, warn)
}

// resolveAgentPID polls briefly for an agent pidfile to avoid killing the wrapper prematurely.
func resolveAgentPID(workerStateDir string, warn func(string)) (int, bool) {
	if strings.TrimSpace(workerStateDir) == "" {
		return 0, false
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		pid, found, err := worker.ReadAgentPID(workerStateDir)
		if err != nil {
			emitKillWarning(warn, fmt.Sprintf("failed to read agent pidfile: %v", err))
		} else if found {
			return pid, true
		}
		time.Sleep(25 * time.Millisecond)
	}
	return 0, false
}

// killPID sends SIGKILL to the provided pid when valid.
func killPID(pid int, warn func(string)) {
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
