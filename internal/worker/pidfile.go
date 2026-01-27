// Package worker provides pidfile helpers for agent process observability.
package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	agentPIDFileName = "agent.pid"
)

var agentPIDCandidates = []string{
	agentPIDFileName,
	"codex.pid",
	"claude.pid",
	"gemini.pid",
}

// ReadAgentPID returns the first valid pid found in the worker state directory.
func ReadAgentPID(workerStateDir string) (int, bool, error) {
	if strings.TrimSpace(workerStateDir) == "" {
		return 0, false, fmt.Errorf("worker state dir is required")
	}
	for _, name := range agentPIDCandidates {
		path := filepath.Join(workerStateDir, name)
		pid, found, err := readPIDFile(path)
		if err != nil {
			return 0, false, err
		}
		if found {
			return pid, true, nil
		}
	}
	return 0, false, nil
}

// readPIDFile reads and parses a pidfile when present.
func readPIDFile(path string) (int, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read pidfile %s: %w", path, err)
	}
	value := strings.TrimSpace(string(data))
	if value == "" {
		return 0, false, nil
	}
	pid, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, fmt.Errorf("parse pidfile %s: %w", path, err)
	}
	if pid <= 0 {
		return 0, false, fmt.Errorf("pidfile %s contains invalid pid %d", path, pid)
	}
	return pid, true, nil
}
