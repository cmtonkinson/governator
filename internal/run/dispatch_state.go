// Package run provides helpers for reading worker dispatch metadata.
package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ReadDispatchWrapperPID loads the wrapper PID from a worker state directory when present.
func ReadDispatchWrapperPID(workerStateDir string) (int, bool, error) {
	if strings.TrimSpace(workerStateDir) == "" {
		return 0, false, nil
	}
	data, err := os.ReadFile(filepath.Join(workerStateDir, "dispatch.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("read dispatch metadata in %s: %w", workerStateDir, err)
	}
	var payload struct {
		WrapperPID int `json:"wrapper_pid"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, false, fmt.Errorf("decode dispatch metadata in %s: %w", workerStateDir, err)
	}
	if payload.WrapperPID <= 0 {
		return 0, false, nil
	}
	return payload.WrapperPID, true, nil
}

// readDispatchWrapperPID loads the wrapper PID from a worker state directory when present.
func readDispatchWrapperPID(workerStateDir string) (int, bool) {
	pid, found, _ := ReadDispatchWrapperPID(workerStateDir)
	return pid, found
}
