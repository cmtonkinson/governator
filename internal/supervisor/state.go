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
	localStateDirName   = "_governator/_local-state"
	planningSupervisorDirName  = "planning_supervisor"
	executionSupervisorDirName = "execution_supervisor"
	supervisorStateFile = "state.json"
	supervisorLogFile   = "supervisor.log"
	supervisorDirMode   = 0o755
	supervisorFileMode  = 0o644
)

// SupervisorKind represents the type of supervisor (planning or execution).
type SupervisorKind string

const (
	// SupervisorKindPlanning denotes the planning supervisor.
	SupervisorKindPlanning SupervisorKind = "planning"
	// SupervisorKindExecution denotes the execution supervisor.
	SupervisorKindExecution SupervisorKind = "execution"
)

// PlanningSupervisorLockName is the lockfile name for planning supervision.
const PlanningSupervisorLockName = "planning_supervisor.lock"
// ExecutionSupervisorLockName is the lockfile name for execution supervision.
const ExecutionSupervisorLockName = "execution_supervisor.lock"

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

// SupervisorStateInfo captures persisted supervisor metadata, unified for both planning and execution.
type SupervisorStateInfo struct {
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

// PlanningSupervisorState is a type alias for SupervisorStateInfo.
type PlanningSupervisorState = SupervisorStateInfo

// ExecutionSupervisorState is a type alias for SupervisorStateInfo.
type ExecutionSupervisorState = SupervisorStateInfo

// ErrSupervisorNotRunning indicates no supervisor is active for the given kind.
var ErrSupervisorNotRunning = errors.New("supervisor not running")

// StatePath returns the path to the supervisor state file for a given kind.
func StatePath(repoRoot string, kind SupervisorKind) string {
	return filepath.Join(repoRoot, localStateDirName, supervisorDirName(kind), supervisorStateFile)
}

// LogPath returns the path to the supervisor log file for a given kind.
func LogPath(repoRoot string, kind SupervisorKind) string {
	return filepath.Join(repoRoot, localStateDirName, supervisorDirName(kind), supervisorLogFile)
}

// LoadState reads the supervisor state for a given kind when present.
func LoadState(repoRoot string, kind SupervisorKind) (SupervisorStateInfo, bool, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return SupervisorStateInfo{}, false, errors.New("repo root is required")
	}
	path := StatePath(repoRoot, kind)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SupervisorStateInfo{}, false, nil
		}
		return SupervisorStateInfo{}, false, fmt.Errorf("read %s supervisor state %s: %w", kind, path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return SupervisorStateInfo{}, false, nil
	}
	var state SupervisorStateInfo
	if err := json.Unmarshal(data, &state); err != nil {
		return SupervisorStateInfo{}, false, fmt.Errorf("decode %s supervisor state %s: %w", kind, path, err)
	}
	return state, true, nil
}

// SaveState persists the supervisor state for a given kind to disk.
func SaveState(repoRoot string, kind SupervisorKind, state SupervisorStateInfo) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
	path := StatePath(repoRoot, kind)
	if err := os.MkdirAll(filepath.Dir(path), supervisorDirMode); err != nil {
		return fmt.Errorf("create %s supervisor directory %s: %w", kind, filepath.Dir(path), err)
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s supervisor state: %w", kind, err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, supervisorFileMode); err != nil {
		return fmt.Errorf("write %s supervisor state %s: %w", kind, path, err)
	}
	return nil
}

// ClearState removes persisted supervisor state and logs for a given kind.
func ClearState(repoRoot string, kind SupervisorKind) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
	dir := filepath.Join(repoRoot, localStateDirName, supervisorDirName(kind))
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s supervisor state %s: %w", kind, dir, err)
	}
	return nil
}

// SupervisorRunning reports whether a supervisor process of a given kind is active.
func SupervisorRunning(repoRoot string, kind SupervisorKind) (SupervisorStateInfo, bool, error) {
	state, ok, err := LoadState(repoRoot, kind)
	if err != nil {
		return SupervisorStateInfo{}, false, err
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

// Thin wrappers for backward compatibility //

// PlanningStatePath returns the path to the planning supervisor state file.
func PlanningStatePath(repoRoot string) string {
	return StatePath(repoRoot, SupervisorKindPlanning)
}

// PlanningLogPath returns the path to the planning supervisor log file.
func PlanningLogPath(repoRoot string) string {
	return LogPath(repoRoot, SupervisorKindPlanning)
}

// ExecutionStatePath returns the path to the execution supervisor state file.
func ExecutionStatePath(repoRoot string) string {
	return StatePath(repoRoot, SupervisorKindExecution)
}

// ExecutionLogPath returns the path to the execution supervisor log file.
func ExecutionLogPath(repoRoot string) string {
	return LogPath(repoRoot, SupervisorKindExecution)
}

// LoadPlanningState reads the planning supervisor state when present.
func LoadPlanningState(repoRoot string) (PlanningSupervisorState, bool, error) {
	state, ok, err := LoadState(repoRoot, SupervisorKindPlanning)
	return PlanningSupervisorState(state), ok, err
}

// LoadExecutionState reads the execution supervisor state when present.
func LoadExecutionState(repoRoot string) (ExecutionSupervisorState, bool, error) {
	state, ok, err := LoadState(repoRoot, SupervisorKindExecution)
	return ExecutionSupervisorState(state), ok, err
}

// SavePlanningState persists the planning supervisor state to disk.
func SavePlanningState(repoRoot string, state PlanningSupervisorState) error {
	return SaveState(repoRoot, SupervisorKindPlanning, SupervisorStateInfo(state))
}

// SaveExecutionState persists the execution supervisor state to disk.
func SaveExecutionState(repoRoot string, state ExecutionSupervisorState) error {
	return SaveState(repoRoot, SupervisorKindExecution, SupervisorStateInfo(state))
}

// ClearPlanningState removes persisted planning supervisor state and logs.
func ClearPlanningState(repoRoot string) error {
	return ClearState(repoRoot, SupervisorKindPlanning)
}

// ClearExecutionState removes persisted execution supervisor state and logs.
func ClearExecutionState(repoRoot string) error {
	return ClearState(repoRoot, SupervisorKindExecution)
}

// PlanningSupervisorRunning reports whether a planning supervisor process is active.
func PlanningSupervisorRunning(repoRoot string) (PlanningSupervisorState, bool, error) {
	state, ok, err := SupervisorRunning(repoRoot, SupervisorKindPlanning)
	return PlanningSupervisorState(state), ok, err
}

// ExecutionSupervisorRunning reports whether an execution supervisor process is active.
func ExecutionSupervisorRunning(repoRoot string) (ExecutionSupervisorState, bool, error) {
	state, ok, err := SupervisorRunning(repoRoot, SupervisorKindExecution)
	return ExecutionSupervisorState(state), ok, err
}

// AnySupervisorRunning reports whether any supervisor process is active.
func AnySupervisorRunning(repoRoot string) (string, bool, error) {
	planningState, planningRunning, err := SupervisorRunning(repoRoot, SupervisorKindPlanning)
	if err != nil {
		return "", false, err
	}
	executionState, executionRunning, err := SupervisorRunning(repoRoot, SupervisorKindExecution)
	if err != nil {
		return "", false, err
	}
	if planningRunning && executionRunning {
		return "", false, fmt.Errorf("multiple supervisors detected: planning pid %d, execution pid %d", planningState.PID, executionState.PID)
	}
	if planningRunning {
		return string(SupervisorKindPlanning), true, nil
	}
	if executionRunning {
		return string(SupervisorKindExecution), true, nil
	}
	return "", false, nil
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

func supervisorDirName(kind SupervisorKind) string {
	switch kind {
	case SupervisorKindPlanning:
		return planningSupervisorDirName
	case SupervisorKindExecution:
		return executionSupervisorDirName
	default:
		return string(kind)
	}
}
