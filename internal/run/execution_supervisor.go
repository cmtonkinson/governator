// Package run provides execution supervisor orchestration.
package run

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/supervisorlock"
)

const defaultExecutionSupervisorPollInterval = 2 * time.Second

// ExecutionSupervisorOptions configures the execution supervisor loop.
type ExecutionSupervisorOptions struct {
	Stdout       io.Writer
	Stderr       io.Writer
	PollInterval time.Duration
	LogPath      string
}

// RunExecutionSupervisor runs the execution supervisor loop until execution completes or fails.
func RunExecutionSupervisor(repoRoot string, opts ExecutionSupervisorOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultExecutionSupervisorPollInterval
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	if held, err := supervisorlock.Held(repoRoot, supervisor.PlanningSupervisorLockName); err != nil {
		return err
	} else if held {
		return errors.New("planning supervisor already running; stop it before starting execution")
	}
	lock, err := supervisorlock.Acquire(repoRoot, supervisor.ExecutionSupervisorLockName)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	cfg, err := config.Load(repoRoot, nil, nil)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		return fmt.Errorf("create in-flight store: %w", err)
	}

	state := newExecutionSupervisorState(repoRoot, opts.LogPath)
	if err := supervisor.SaveExecutionState(repoRoot, state); err != nil {
		return err
	}

	stopSignals := make(chan os.Signal, 1)
	signal.Notify(stopSignals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stopSignals)

	for {
		select {
		case <-stopSignals:
			state.State = supervisor.SupervisorStateStopped
			state.Error = ""
			state = MarkSupervisorTransition(state)
			return supervisor.SaveExecutionState(repoRoot, state)
		default:
		}

		idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
		if err != nil {
			return failExecutionSupervisor(repoRoot, &state, fmt.Errorf("load task index: %w", err))
		}
		planning, err := newPlanningTask(repoRoot)
		if err != nil {
			return failExecutionSupervisor(repoRoot, &state, fmt.Errorf("load planning spec: %w", err))
		}
		complete, err := planningComplete(idx, planning)
		if err != nil {
			return failExecutionSupervisor(repoRoot, &state, fmt.Errorf("planning index: %w", err))
		}
		if !complete {
			return failExecutionSupervisor(repoRoot, &state, fmt.Errorf("planning incomplete: run `governator plan` before executing"))
		}

		inFlight, err := inFlightStore.Load()
		if err != nil {
			return failExecutionSupervisor(repoRoot, &state, fmt.Errorf("load in-flight tasks: %w", err))
		}
		if inFlight == nil {
			inFlight = inflight.Set{}
		}
		backlogCount := countBacklog(idx)
		if backlogCount > 0 {
			if len(inFlight) > 0 {
				state.StepID = "drain"
				state.StepName = "Drain"
				if err := maybePersistExecutionSupervisorState(repoRoot, &state); err != nil {
					return err
				}
				if _, err := Run(repoRoot, Options{Stdout: stdout, Stderr: stderr, DisableDispatch: true}); err != nil {
					return failExecutionSupervisor(repoRoot, &state, err)
				}
				time.Sleep(opts.PollInterval)
				continue
			}

			state.StepID = "triage"
			state.StepName = "Triage"
			if err := maybePersistExecutionSupervisorState(repoRoot, &state); err != nil {
				return err
			}
			triageResult, err := RunBacklogTriage(repoRoot, &idx, cfg, Options{Stdout: stdout, Stderr: stderr})
			if err != nil {
				return failExecutionSupervisor(repoRoot, &state, err)
			}
			if triageResult.Running {
				state.WorkerPID = triageResult.WorkerPID
				state.WorkerStateDir = triageResult.WorkerStateDir
				if err := maybePersistExecutionSupervisorState(repoRoot, &state); err != nil {
					return err
				}
			}
			time.Sleep(opts.PollInterval)
			continue
		}

		done, err := executionComplete(idx)
		if err != nil {
			return failExecutionSupervisor(repoRoot, &state, err)
		}
		if done {
			return completeExecutionSupervisor(repoRoot, &state)
		}

		state.StepID = "execution"
		state.StepName = "Execution"

		if err := maybePersistExecutionSupervisorState(repoRoot, &state); err != nil {
			return err
		}

		if _, err := Run(repoRoot, Options{Stdout: stdout, Stderr: stderr}); err != nil {
			return failExecutionSupervisor(repoRoot, &state, err)
		}

		time.Sleep(opts.PollInterval)
	}
}

// newExecutionSupervisorState builds the initial execution supervisor state payload.
func newExecutionSupervisorState(repoRoot string, logPath string) supervisor.ExecutionSupervisorState {
	if strings.TrimSpace(logPath) == "" {
		logPath = supervisor.ExecutionLogPath(repoRoot)
	}
	now := time.Now().UTC()
	return supervisor.ExecutionSupervisorState{
		Phase:          "execution",
		PID:            os.Getpid(),
		State:          supervisor.SupervisorStateRunning,
		StartedAt:      now,
		LastTransition: now,
		LogPath:        logPath,
	}
}

// completeExecutionSupervisor clears persisted supervisor state after a healthy completion.
func completeExecutionSupervisor(repoRoot string, state *supervisor.ExecutionSupervisorState) error {
	if state == nil {
		return errors.New("execution supervisor state is required")
	}
	state.State = supervisor.SupervisorStateCompleted
	state.StepID = ""
	state.StepName = ""
	state.WorkerPID = 0
	state.ValidationPID = 0
	state.WorkerStateDir = ""
	state.Error = ""
	updated := MarkSupervisorTransition(*state)
	*state = updated
	if err := supervisor.ClearExecutionState(repoRoot); err != nil {
		if saveErr := supervisor.SaveExecutionState(repoRoot, updated); saveErr != nil {
			return fmt.Errorf("clear execution supervisor state: %w; save fallback failed: %v", err, saveErr)
		}
		return fmt.Errorf("clear execution supervisor state: %w", err)
	}
	return nil
}

// maybePersistExecutionSupervisorState persists state changes when data differs from disk.
func maybePersistExecutionSupervisorState(repoRoot string, state *supervisor.ExecutionSupervisorState) error {
	if state == nil {
		return errors.New("execution supervisor state is required")
	}
	current, _, err := supervisor.LoadExecutionState(repoRoot)
	if err != nil {
		return err
	}
	if SupervisorStateEqual(current, *state) {
		return nil
	}
	state.LastTransition = time.Now().UTC()
	return supervisor.SaveExecutionState(repoRoot, *state)
}

// countBacklog returns the number of execution tasks awaiting triage.
func countBacklog(idx index.Index) int {
	count := 0
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State == index.TaskStateBacklog {
			count++
		}
	}
	return count
}

// failExecutionSupervisor persists failure metadata and returns the root error.
func failExecutionSupervisor(repoRoot string, state *supervisor.ExecutionSupervisorState, err error) error {
	if state == nil {
		return err
	}
	state.State = supervisor.SupervisorStateFailed
	state.Error = err.Error()
	updated := MarkSupervisorTransition(*state)
	*state = updated
	if saveErr := supervisor.SaveExecutionState(repoRoot, updated); saveErr != nil {
		return fmt.Errorf("%w; supervisor state save failed: %v", err, saveErr)
	}
	return err
}

// executionComplete reports whether all execution tasks are in terminal states.
func executionComplete(idx index.Index) (bool, error) {
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if !executionTerminalState(task.State) {
			return false, nil
		}
	}
	return true, nil
}

// executionTerminalState reports whether a task state is terminal for execution.
func executionTerminalState(state index.TaskState) bool {
	switch state {
	case index.TaskStateMerged, index.TaskStateBlocked, index.TaskStateConflict:
		return true
	default:
		return false
	}
}
