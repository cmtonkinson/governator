// Package run provides helpers for reading worker dispatch metadata.
package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// readDispatchWrapperPID loads the wrapper PID from a worker state directory when present.
func readDispatchWrapperPID(workerStateDir string) (int, bool) {
	if strings.TrimSpace(workerStateDir) == "" {
		return 0, false
	}
	data, err := os.ReadFile(filepath.Join(workerStateDir, "dispatch.json"))
	if err != nil {
		return 0, false
	}
	var payload struct {
		WrapperPID int `json:"wrapper_pid"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0, false
	}
	if payload.WrapperPID <= 0 {
		return 0, false
	}
	return payload.WrapperPID, true
}
