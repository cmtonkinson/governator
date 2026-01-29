// Package supervisor provides state tracking for governator supervisors.
package supervisor

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	localStateDirName           = "_governator/_local-state"
	planningSupervisorDirName   = "planning_supervisor"
	planningSupervisorStateFile = "state.json"
	planningSupervisorLogFile   = "supervisor.log"
	planningSupervisorDirMode   = 0o755
	planningSupervisorFileMode  = 0o644
)

// SupervisorState labels the lifecycle state for a supervisor.
type SupervisorState string

const (
	// SupervisorStateRunning indicates the supervisor is active.
	SupervisorStateRunning SupervisorState = "running"
	// SupervisorStateStopped indicates the supervisor was stopped intentionally.
	SupervisorStateStopped SupervisorState = "stopped"
	// SupervisorStateFailed indicates the supervisor exited after an error.
	SupervisorStateFailed SupervisorState = "failed"
	// SupervisorStateCompleted indicates the supervisor completed its workstream.
	SupervisorStateCompleted SupervisorState = "completed"
)

// PlanningSupervisorState captures persisted planning supervisor metadata.
type PlanningSupervisorState struct {
	Phase          string          `json:"phase"`
	PID            int             `json:"pid"`
	WorkerPID      int             `json:"worker_pid,omitempty"`
	ValidationPID  int             `json:"validation_pid,omitempty"`
	StepID         string          `json:"step_id,omitempty"`
	StepName       string          `json:"step_name,omitempty"`
	State          SupervisorState `json:"state"`
	StartedAt      time.Time       `json:"started_at"`
	LastTransition time.Time       `json:"last_transition"`
	LogPath        string          `json:"log_path,omitempty"`
	Error          string          `json:"error,omitempty"`
	WorkerStateDir string          `json:"worker_state_dir,omitempty"`
}

// ErrPlanningSupervisorNotRunning indicates no planning supervisor is active.
var ErrPlanningSupervisorNotRunning = errors.New("planning supervisor not running")

// PlanningStatePath returns the path to the planning supervisor state file.
func PlanningStatePath(repoRoot string) string {
	return filepath.Join(repoRoot, localStateDirName, planningSupervisorDirName, planningSupervisorStateFile)
}

// PlanningLogPath returns the path to the planning supervisor log file.
func PlanningLogPath(repoRoot string) string {
	return filepath.Join(repoRoot, localStateDirName, planningSupervisorDirName, planningSupervisorLogFile)
}

// LoadPlanningState reads the planning supervisor state when present.
func LoadPlanningState(repoRoot string) (PlanningSupervisorState, bool, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return PlanningSupervisorState{}, false, errors.New("repo root is required")
	}
	path := PlanningStatePath(repoRoot)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PlanningSupervisorState{}, false, nil
		}
		return PlanningSupervisorState{}, false, fmt.Errorf("read planning supervisor state %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return PlanningSupervisorState{}, false, nil
	}
	var state PlanningSupervisorState
	if err := json.Unmarshal(data, &state); err != nil {
		return PlanningSupervisorState{}, false, fmt.Errorf("decode planning supervisor state %s: %w", path, err)
	}
	return state, true, nil
}

// SavePlanningState persists the planning supervisor state to disk.
func SavePlanningState(repoRoot string, state PlanningSupervisorState) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
	path := PlanningStatePath(repoRoot)
	if err := os.MkdirAll(filepath.Dir(path), planningSupervisorDirMode); err != nil {
		return fmt.Errorf("create planning supervisor directory %s: %w", filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode planning supervisor state: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, planningSupervisorFileMode); err != nil {
		return fmt.Errorf("write planning supervisor state %s: %w", path, err)
	}
	return nil
}

// ClearPlanningState removes persisted planning supervisor state and logs.
func ClearPlanningState(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
	dir := filepath.Join(repoRoot, localStateDirName, planningSupervisorDirName)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove planning supervisor state %s: %w", dir, err)
	}
	return nil
}

// PlanningSupervisorRunning reports whether a planning supervisor process is active.
func PlanningSupervisorRunning(repoRoot string) (PlanningSupervisorState, bool, error) {
	state, ok, err := LoadPlanningState(repoRoot)
	if err != nil {
		return PlanningSupervisorState{}, false, err
	}
	if !ok || state.PID <= 0 {
		return state, false, nil
	}
	alive, err := processExists(state.PID)
	if err != nil {
		return state, false, err
	}
	return state, alive, nil
}

func processExists(pid int) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	err := syscall.Kill(pid, 0)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return false, err
}
