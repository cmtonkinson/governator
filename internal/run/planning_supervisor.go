// Package run provides planning supervisor orchestration.
package run

import (
	"encoding/json"
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
	"github.com/cmtonkinson/governator/internal/worker"
)

const defaultPlanningSupervisorPollInterval = 2 * time.Second

// PlanningSupervisorOptions configures the planning supervisor loop.
type PlanningSupervisorOptions struct {
	Stdout       io.Writer
	Stderr       io.Writer
	PollInterval time.Duration
	LogPath      string
}

// RunPlanningSupervisor runs the planning supervisor loop until planning completes or fails.
func RunPlanningSupervisor(repoRoot string, opts PlanningSupervisorOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	if opts.PollInterval <= 0 {
		opts.PollInterval = defaultPlanningSupervisorPollInterval
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}

	cfg, err := config.Load(repoRoot, nil, nil)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		return fmt.Errorf("load planning spec: %w", err)
	}
	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		return fmt.Errorf("create in-flight store: %w", err)
	}

	state := newPlanningSupervisorState(repoRoot, opts.LogPath)
	if err := supervisor.SavePlanningState(repoRoot, state); err != nil {
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
			state = markPlanningSupervisorTransition(state)
			return supervisor.SavePlanningState(repoRoot, state)
		default:
		}

		idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
		if err != nil {
			return failPlanningSupervisor(repoRoot, &state, fmt.Errorf("load task index: %w", err))
		}

		complete, err := planningComplete(idx, planning)
		if err != nil {
			return failPlanningSupervisor(repoRoot, &state, fmt.Errorf("planning index: %w", err))
		}
		if complete {
			return completePlanningSupervisor(repoRoot, &state)
		}

		step, stepOK, err := currentPlanningStep(idx, planning)
		if err != nil {
			return failPlanningSupervisor(repoRoot, &state, fmt.Errorf("planning step: %w", err))
		}
		if !stepOK {
			return failPlanningSupervisor(repoRoot, &state, fmt.Errorf("planning step missing while planning is incomplete"))
		}
		if stepOK {
			state.StepID = step.name
			state.StepName = step.title()
		}

		inFlight, err := inFlightStore.Load()
		if err != nil {
			return failPlanningSupervisor(repoRoot, &state, fmt.Errorf("load in-flight tasks: %w", err))
		}
		if inFlight == nil {
			inFlight = inflight.Set{}
		}
		if stepOK {
			state = refreshPlanningWorkerState(state, inFlight, step)
		}
		if err := maybePersistPlanningSupervisorState(repoRoot, &state); err != nil {
			return err
		}

		phaseRunner := newPhaseRunner(repoRoot, cfg, Options{Stdout: stdout, Stderr: stderr}, inFlightStore, inFlight, planning)
		if _, err := phaseRunner.EnsurePlanningPhases(&idx); err != nil {
			return failPlanningSupervisor(repoRoot, &state, err)
		}

		time.Sleep(opts.PollInterval)
	}
}

func newPlanningSupervisorState(repoRoot string, logPath string) supervisor.PlanningSupervisorState {
	if strings.TrimSpace(logPath) == "" {
		logPath = supervisor.PlanningLogPath(repoRoot)
	}
	now := time.Now().UTC()
	return supervisor.PlanningSupervisorState{
		Phase:          "planning",
		PID:            os.Getpid(),
		State:          supervisor.SupervisorStateRunning,
		StartedAt:      now,
		LastTransition: now,
		LogPath:        logPath,
	}
}

func refreshPlanningWorkerState(state supervisor.PlanningSupervisorState, inFlight inflight.Set, step workstreamStep) supervisor.PlanningSupervisorState {
	entry, ok := inFlight.Entry(step.workstreamID())
	if !ok {
		state.WorkerPID = 0
		state.WorkerStateDir = ""
		return state
	}
	state.WorkerStateDir = entry.WorkerStateDir
	pid, found, err := worker.ReadAgentPID(entry.WorkerStateDir)
	if err == nil && found {
		state.WorkerPID = pid
		return state
	}
	wrapperPID, ok := readDispatchWrapperPID(entry.WorkerStateDir)
	if ok {
		state.WorkerPID = wrapperPID
	} else {
		state.WorkerPID = 0
	}
	return state
}

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

func markPlanningSupervisorTransition(state supervisor.PlanningSupervisorState) supervisor.PlanningSupervisorState {
	state.LastTransition = time.Now().UTC()
	return state
}

// completePlanningSupervisor clears persisted supervisor state after a healthy completion.
func completePlanningSupervisor(repoRoot string, state *supervisor.PlanningSupervisorState) error {
	if state == nil {
		return errors.New("planning supervisor state is required")
	}
	state.State = supervisor.SupervisorStateCompleted
	state.StepID = ""
	state.StepName = ""
	state.WorkerPID = 0
	state.ValidationPID = 0
	state.WorkerStateDir = ""
	state.Error = ""
	updated := markPlanningSupervisorTransition(*state)
	*state = updated
	if err := supervisor.ClearPlanningState(repoRoot); err != nil {
		if saveErr := supervisor.SavePlanningState(repoRoot, updated); saveErr != nil {
			return fmt.Errorf("clear planning supervisor state: %w; save fallback failed: %v", err, saveErr)
		}
		return fmt.Errorf("clear planning supervisor state: %w", err)
	}
	return nil
}

func maybePersistPlanningSupervisorState(repoRoot string, state *supervisor.PlanningSupervisorState) error {
	if state == nil {
		return errors.New("planning supervisor state is required")
	}
	current, _, err := supervisor.LoadPlanningState(repoRoot)
	if err != nil {
		return err
	}
	if planningSupervisorStateEqual(current, *state) {
		return nil
	}
	state.LastTransition = time.Now().UTC()
	return supervisor.SavePlanningState(repoRoot, *state)
}

func planningSupervisorStateEqual(left supervisor.PlanningSupervisorState, right supervisor.PlanningSupervisorState) bool {
	return left.Phase == right.Phase &&
		left.PID == right.PID &&
		left.WorkerPID == right.WorkerPID &&
		left.ValidationPID == right.ValidationPID &&
		left.StepID == right.StepID &&
		left.StepName == right.StepName &&
		left.State == right.State &&
		left.LogPath == right.LogPath &&
		left.Error == right.Error &&
		left.WorkerStateDir == right.WorkerStateDir &&
		left.StartedAt.Equal(right.StartedAt)
}

func failPlanningSupervisor(repoRoot string, state *supervisor.PlanningSupervisorState, err error) error {
	if state == nil {
		return err
	}
	state.State = supervisor.SupervisorStateFailed
	state.Error = err.Error()
	updated := markPlanningSupervisorTransition(*state)
	*state = updated
	if saveErr := supervisor.SavePlanningState(repoRoot, updated); saveErr != nil {
		return fmt.Errorf("%w; supervisor state save failed: %v", err, saveErr)
	}
	return err
}
