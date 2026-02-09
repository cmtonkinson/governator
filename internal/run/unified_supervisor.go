// Package run provides unified supervisor orchestration.
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

const defaultUnifiedSupervisorPollInterval = 2 * time.Second

// UnifiedSupervisorOptions configures the unified supervisor loop.
type UnifiedSupervisorOptions struct {
	Stdout       io.Writer
	Stderr       io.Writer
	PollInterval time.Duration
	LogPath      string
}

var (
	runFunc               = Run
	runBacklogTriageFunc  = RunBacklogTriage
	detectPlanningDriftFn = DetectPlanningDrift
	planningCompleteFunc  = planningComplete
	countBacklogFunc      = countBacklog
	executionCompleteFunc = executionComplete
)

// RunUnifiedSupervisor runs the unified supervisor loop until orchestration completes or fails.
func RunUnifiedSupervisor(repoRoot string, opts UnifiedSupervisorOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultUnifiedSupervisorPollInterval
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	lock, err := supervisorlock.Acquire(repoRoot, supervisor.SupervisorLockName)
	if err != nil {
		return err
	}
	defer func() {
		_ = lock.Release()
	}()

	if err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{}); err != nil {
		return fmt.Errorf("apply repo migrations: %w", err)
	}

	cfg, err := config.Load(repoRoot, nil, nil)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		return fmt.Errorf("create in-flight store: %w", err)
	}

	state := newUnifiedSupervisorState(repoRoot, opts.LogPath)
	if err := supervisor.SaveState(repoRoot, state); err != nil {
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
			return supervisor.SaveState(repoRoot, state)
		default:
		}

		idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
		if err != nil {
			return failUnifiedSupervisor(repoRoot, &state, fmt.Errorf("load task index: %w", err))
		}
		planning, err := newPlanningTask(repoRoot)
		if err != nil {
			return failUnifiedSupervisor(repoRoot, &state, fmt.Errorf("load planning spec: %w", err))
		}
		inFlight, err := inFlightStore.Load()
		if err != nil {
			return failUnifiedSupervisor(repoRoot, &state, fmt.Errorf("load in-flight tasks: %w", err))
		}
		if inFlight == nil {
			inFlight = inflight.Set{}
		}

		planningDrift, err := detectPlanningDriftFn(repoRoot, idx.Digests)
		if err != nil {
			return failUnifiedSupervisor(repoRoot, &state, err)
		}
		if planningDrift.HasDrift {
			state.StepID = "drain"
			state.StepName = "Drain"
			state.WorkerPID = 0
			state.WorkerStateDir = ""
			if err := maybePersistUnifiedSupervisorState(repoRoot, &state); err != nil {
				return err
			}
			emitDriftReplanMessage(stdout, planningDrift.Message)
			if len(inFlight) > 0 {
				if _, err := runFunc(repoRoot, Options{Stdout: stdout, Stderr: stderr, DisableDispatch: true, SkipPlanningDrift: true}); err != nil {
					return failUnifiedSupervisor(repoRoot, &state, err)
				}
				time.Sleep(opts.PollInterval)
				continue
			}
			if err := ResetPlanningToStep(repoRoot, "gap-analysis"); err != nil {
				return failUnifiedSupervisor(repoRoot, &state, err)
			}
			time.Sleep(opts.PollInterval)
			continue
		}

		complete, err := planningCompleteFunc(idx, planning)
		if err != nil {
			return failUnifiedSupervisor(repoRoot, &state, fmt.Errorf("planning index: %w", err))
		}
		if !complete {
			state.StepID = "plan"
			state.StepName = "Plan"
			state.WorkerPID = 0
			state.WorkerStateDir = ""
			if err := maybePersistUnifiedSupervisorState(repoRoot, &state); err != nil {
				return err
			}
			if _, err := runFunc(repoRoot, Options{Stdout: stdout, Stderr: stderr, SkipPlanningDrift: true}); err != nil {
				return failUnifiedSupervisor(repoRoot, &state, err)
			}
			time.Sleep(opts.PollInterval)
			continue
		}

		backlogCount := countBacklogFunc(idx)
		if backlogCount > 0 {
			if len(inFlight) > 0 {
				state.StepID = "drain"
				state.StepName = "Drain"
				if err := maybePersistUnifiedSupervisorState(repoRoot, &state); err != nil {
					return err
				}
				if _, err := runFunc(repoRoot, Options{Stdout: stdout, Stderr: stderr, DisableDispatch: true, SkipPlanningDrift: true}); err != nil {
					return failUnifiedSupervisor(repoRoot, &state, err)
				}
				time.Sleep(opts.PollInterval)
				continue
			}

			state.StepID = "triage"
			state.StepName = "Triage"
			if err := maybePersistUnifiedSupervisorState(repoRoot, &state); err != nil {
				return err
			}
			triageResult, err := runBacklogTriageFunc(repoRoot, &idx, cfg, Options{Stdout: stdout, Stderr: stderr})
			if err != nil {
				return failUnifiedSupervisor(repoRoot, &state, err)
			}
			if triageResult.Running {
				state.WorkerPID = triageResult.WorkerPID
				state.WorkerStateDir = triageResult.WorkerStateDir
				if err := maybePersistUnifiedSupervisorState(repoRoot, &state); err != nil {
					return err
				}
			}
			time.Sleep(opts.PollInterval)
			continue
		}

		done, err := executionCompleteFunc(idx)
		if err != nil {
			return failUnifiedSupervisor(repoRoot, &state, err)
		}
		if done {
			return completeUnifiedSupervisor(repoRoot, &state)
		}

		state.StepID = "execute"
		state.StepName = "Execute"
		if err := maybePersistUnifiedSupervisorState(repoRoot, &state); err != nil {
			return err
		}
		if _, err := runFunc(repoRoot, Options{Stdout: stdout, Stderr: stderr, SkipPlanningDrift: true}); err != nil {
			return failUnifiedSupervisor(repoRoot, &state, err)
		}

		time.Sleep(opts.PollInterval)
	}
}

// newUnifiedSupervisorState builds the initial unified supervisor state payload.
func newUnifiedSupervisorState(repoRoot string, logPath string) supervisor.SupervisorStateInfo {
	if strings.TrimSpace(logPath) == "" {
		logPath = supervisor.LogPath(repoRoot)
	}
	now := time.Now().UTC()
	return supervisor.SupervisorStateInfo{
		Phase:          "start",
		PID:            os.Getpid(),
		State:          supervisor.SupervisorStateRunning,
		StartedAt:      now,
		LastTransition: now,
		LogPath:        logPath,
	}
}

// completeUnifiedSupervisor clears persisted supervisor state after a healthy completion.
func completeUnifiedSupervisor(repoRoot string, state *supervisor.SupervisorStateInfo) error {
	if state == nil {
		return errors.New("unified supervisor state is required")
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
	if err := supervisor.ClearState(repoRoot); err != nil {
		if saveErr := supervisor.SaveState(repoRoot, updated); saveErr != nil {
			return fmt.Errorf("clear supervisor state: %w; save fallback failed: %v", err, saveErr)
		}
		return fmt.Errorf("clear supervisor state: %w", err)
	}
	return nil
}

// maybePersistUnifiedSupervisorState persists state changes when data differs from disk.
func maybePersistUnifiedSupervisorState(repoRoot string, state *supervisor.SupervisorStateInfo) error {
	if state == nil {
		return errors.New("unified supervisor state is required")
	}
	current, _, err := supervisor.LoadState(repoRoot)
	if err != nil {
		return err
	}
	if SupervisorStateEqual(current, *state) {
		return nil
	}
	state.LastTransition = time.Now().UTC()
	return supervisor.SaveState(repoRoot, *state)
}

// failUnifiedSupervisor persists failure metadata and returns the root error.
func failUnifiedSupervisor(repoRoot string, state *supervisor.SupervisorStateInfo, err error) error {
	if state == nil {
		return err
	}
	state.State = supervisor.SupervisorStateFailed
	state.Error = err.Error()
	updated := MarkSupervisorTransition(*state)
	*state = updated
	if saveErr := supervisor.SaveState(repoRoot, updated); saveErr != nil {
		return fmt.Errorf("%w; supervisor state save failed: %v", err, saveErr)
	}
	return err
}
