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
	supervisorDirName   = "supervisor"
	supervisorStateFile = "state.json"
	supervisorLogFile   = "supervisor.log"
	supervisorDirMode   = 0o755
	supervisorFileMode  = 0o644
)

// SupervisorLockName is the lockfile name for unified supervision.
const SupervisorLockName = "supervisor.lock"

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

// SupervisorStateInfo captures persisted supervisor metadata.
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

// ErrSupervisorNotRunning indicates no supervisor is active.
var ErrSupervisorNotRunning = errors.New("supervisor not running")

// StatePath returns the path to the unified supervisor state file.
func StatePath(repoRoot string) string {
	return filepath.Join(repoRoot, localStateDirName, supervisorDirName, supervisorStateFile)
}

// LogPath returns the path to the unified supervisor log file.
func LogPath(repoRoot string) string {
	return filepath.Join(repoRoot, localStateDirName, supervisorDirName, supervisorLogFile)
}

// LoadState reads the persisted supervisor state when present.
func LoadState(repoRoot string) (SupervisorStateInfo, bool, error) {
	return loadSupervisorState(repoRoot, StatePath(repoRoot), "supervisor")
}

// SaveState persists the supervisor state to disk.
func SaveState(repoRoot string, state SupervisorStateInfo) error {
	return saveSupervisorState(repoRoot, StatePath(repoRoot), "supervisor", state)
}

// ClearState removes persisted supervisor state and logs.
func ClearState(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
	dir := filepath.Join(repoRoot, localStateDirName, supervisorDirName)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove supervisor state %s: %w", dir, err)
	}
	return nil
}

// SupervisorRunning reports whether the unified supervisor process is active.
func SupervisorRunning(repoRoot string) (SupervisorStateInfo, bool, error) {
	state, ok, err := LoadState(repoRoot)
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

// AnyRunning reports whether any supervisor process is active.
func AnyRunning(repoRoot string) (string, bool, error) {
	if _, running, err := SupervisorRunning(repoRoot); err != nil {
		return "", false, err
	} else if running {
		return "supervisor", true, nil
	}
	return "", false, nil
}

func loadSupervisorState(repoRoot, path, kind string) (SupervisorStateInfo, bool, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return SupervisorStateInfo{}, false, errors.New("repo root is required")
	}
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

func saveSupervisorState(repoRoot, path, kind string, state SupervisorStateInfo) error {
	if strings.TrimSpace(repoRoot) == "" {
		return errors.New("repo root is required")
	}
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
