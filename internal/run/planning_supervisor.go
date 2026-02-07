// Package run provides planning supervisor orchestration.
package run

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/supervisorlock"
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

	if held, err := supervisorlock.Held(repoRoot, supervisor.ExecutionSupervisorLockName); err != nil {
		return err
	} else if held {
		return errors.New("execution supervisor already running; stop it before starting planning")
	}
	lock, err := supervisorlock.Acquire(repoRoot, supervisor.PlanningSupervisorLockName)
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
			state = MarkSupervisorTransition(state)
			return supervisor.SavePlanningState(repoRoot, state)
		default:
		}

		idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
		if err != nil {
			return failPlanningSupervisor(repoRoot, &state, fmt.Errorf("load task index: %w", err))
		}

		inFlight, err := inFlightStore.Load()
		if err != nil {
			return failPlanningSupervisor(repoRoot, &state, fmt.Errorf("load in-flight tasks: %w", err))
		}
		if inFlight == nil {
			inFlight = inflight.Set{}
		}

		handled, err := maybeCompletePlanningFromTasks(repoRoot, planning, &idx, inFlight)
		if err != nil {
			return failPlanningSupervisor(repoRoot, &state, err)
		}
		if handled {
			return completePlanningSupervisor(repoRoot, &state)
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

// maybeCompletePlanningFromTasks registers on-disk tasks and completes planning when the plan is unused.
func maybeCompletePlanningFromTasks(repoRoot string, planning planningTask, idx *index.Index, inFlight inflight.Set) (bool, error) {
	if idx == nil {
		return false, errors.New("task index is required")
	}
	stateID, err := planningTaskState(*idx)
	if err != nil {
		return false, err
	}

	notStarted := stateID == PlanningNotStartedState && !inFlight.Contains(planningIndexTaskID)
	alreadyComplete := stateID == PlanningCompleteState
	if !notStarted && !alreadyComplete {
		return false, nil
	}

	needsInventory, err := tasksMissingFromIndex(repoRoot, *idx)
	if err != nil {
		return false, err
	}
	if !needsInventory {
		return false, nil
	}

	taskInventory := NewTaskInventory(repoRoot, idx)
	inventoryResult, err := taskInventory.InventoryTasks()
	if err != nil {
		return false, fmt.Errorf("task inventory failed: %w", err)
	}
	if inventoryResult.TasksAdded == 0 {
		return false, nil
	}

	updatePlanningTaskState(idx, "")
	digestsMap, err := digests.Compute(repoRoot)
	if err != nil {
		return false, fmt.Errorf("compute digests: %w", err)
	}
	idx.Digests = digestsMap

	indexPath := filepath.Join(repoRoot, indexFilePath)
	if err := index.Save(indexPath, *idx); err != nil {
		return false, fmt.Errorf("save task index: %w", err)
	}
	return true, nil
}

// tasksMissingFromIndex reports whether any on-disk task file is missing from the index.
func tasksMissingFromIndex(repoRoot string, idx index.Index) (bool, error) {
	tasksDir := filepath.Join(repoRoot, "_governator", "tasks")
	if _, err := os.Stat(tasksDir); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat tasks directory: %w", err)
	}
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return false, fmt.Errorf("read tasks directory: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join("_governator", "tasks", entry.Name())
		found := false
		for _, task := range idx.Tasks {
			if task.Path == path {
				found = true
				break
			}
		}
		if !found {
			return true, nil
		}
	}
	return false, nil
}

// firstPlanningStepID returns the first step id from the planning spec.
func firstPlanningStepID(planning planningTask) (string, bool) {
	if len(planning.ordered) == 0 {
		return "", false
	}
	return planning.ordered[0].name, true
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
	updated := MarkSupervisorTransition(*state)
	*state = updated
	if err := supervisor.ClearPlanningState(repoRoot); err != nil {
		if saveErr := supervisor.SavePlanningState(repoRoot, updated); saveErr != nil {
			return fmt.Errorf("clear planning supervisor state: %w; save fallback failed: %v", err, saveErr)
		}
		return fmt.Errorf("clear planning supervisor state: %w", err)
	}

	// Clean up planning branch and worktree after state is cleared
	if err := cleanupPlanningBranch(repoRoot); err != nil {
		// Log to stderr but don't fail completion - cleanup is non-critical
		// Planning state is already cleared, so completion succeeded
		fmt.Fprintf(os.Stderr, "Warning: failed to cleanup planning branch: %v\n", err)
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
	if SupervisorStateEqual(current, *state) {
		return nil
	}
	state.LastTransition = time.Now().UTC()
	return supervisor.SavePlanningState(repoRoot, *state)
}



func failPlanningSupervisor(repoRoot string, state *supervisor.PlanningSupervisorState, err error) error {
	if state == nil {
		return err
	}
	state.State = supervisor.SupervisorStateFailed
	state.Error = err.Error()
	updated := MarkSupervisorTransition(*state)
	*state = updated
	if saveErr := supervisor.SavePlanningState(repoRoot, updated); saveErr != nil {
		return fmt.Errorf("%w; supervisor state save failed: %v", err, saveErr)
	}
	return err
}

// cleanupPlanningBranch removes the planning worktree and branch after planning completes.
func cleanupPlanningBranch(repoRoot string) error {
	planningBranch := planningIndexTaskID // "planning" constant
	worktreePath := filepath.Join(repoRoot, "_governator", "_local-state", "task-planning")

	// Step 1: Remove the planning worktree if it exists
	if _, err := os.Stat(worktreePath); err == nil {
		if err := runGitInRepo(repoRoot, "worktree", "remove", "--force", worktreePath); err != nil {
			return fmt.Errorf("remove planning worktree: %w", err)
		}
	}

	// Step 2: Delete the planning branch if it exists
	// Check if branch exists first to avoid errors
	checkCmd := exec.Command("git", "rev-parse", "--verify", planningBranch)
	checkCmd.Dir = repoRoot
	if checkCmd.Run() == nil {
		// Branch exists, delete it
		if err := runGitInRepo(repoRoot, "branch", "-D", planningBranch); err != nil {
			return fmt.Errorf("delete planning branch: %w", err)
		}
	}

	return nil
}
