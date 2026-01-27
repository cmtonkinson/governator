// Package run implements Governator run orchestration helpers.
package run

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/phase"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/worker"
	"github.com/cmtonkinson/governator/internal/worktree"
)

type phaseRunner struct {
	repoRoot            string
	cfg                 config.Config
	stdout              io.Writer
	stderr              io.Writer
	store               *phase.Store
	worktreeManager     worktree.Manager
	worktreeManagerInit bool
}

func newPhaseRunner(repoRoot string, cfg config.Config, opts Options, store *phase.Store) *phaseRunner {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return &phaseRunner{
		repoRoot: repoRoot,
		cfg:      cfg,
		stdout:   stdout,
		stderr:   stderr,
		store:    store,
	}
}

func (runner *phaseRunner) ensureWorktreeManager() error {
	if runner.worktreeManagerInit {
		return nil
	}
	manager, err := worktree.NewManager(runner.repoRoot)
	if err != nil {
		return err
	}
	runner.worktreeManager = manager
	runner.worktreeManagerInit = true
	return nil
}

func (runner *phaseRunner) EnsurePlanningPhases(state *phase.State) (bool, error) {
	if err := runner.ensureWorktreeManager(); err != nil {
		return false, fmt.Errorf("create worktree manager: %w", err)
	}
	for state.Current < phase.PhaseExecution {
		spec, ok := planningPhaseSpecs[state.Current]
		if !ok {
			return false, fmt.Errorf("unsupported phase %s", state.Current)
		}
		record := state.RecordFor(state.Current)

		if record.Agent.PID != 0 && record.Agent.FinishedAt.IsZero() {
			if runner.isProcessAlive(record.Agent.PID) {
				runner.emitPhaseRunning(state.Current, record.Agent.PID)
				return true, nil
			}
			if err := runner.collectPhaseCompletion(spec); err != nil {
				return false, err
			}
			record.Agent.FinishedAt = runner.now()
			state.SetRecord(state.Current, record)
			if err := runner.store.Save(*state); err != nil {
				return false, fmt.Errorf("save phase state: %w", err)
			}
			runner.emitPhaseAgentComplete(state.Current, record.Agent.PID)
		}

		if record.Agent.PID != 0 && !record.Agent.FinishedAt.IsZero() {
			if err := runner.completePhase(state); err != nil {
				return false, err
			}
			continue
		}

		if err := runner.ensurePhasePrereqs(state.Current); err != nil {
			return false, err
		}

		if err := runner.dispatchPhase(state, spec); err != nil {
			return false, err
		}
		return true, nil
	}

	return false, nil
}

func (runner *phaseRunner) dispatchPhase(state *phase.State, spec phaseSpec) error {
	taskID := fmt.Sprintf("phase-%s", spec.phase.String())
	task := index.Task{
		ID:   taskID,
		Path: spec.promptPath,
		Role: spec.role,
	}

	branchName := fmt.Sprintf("phase-%s", spec.phase.String())
	baseBranch := strings.TrimSpace(runner.cfg.Branches.Base)
	if baseBranch == "" {
		baseBranch = config.Defaults().Branches.Base
	}
	worktreeResult, err := runner.worktreeManager.EnsureWorktree(worktree.Spec{
		WorkstreamID: taskID,
		Branch:       branchName,
		BaseBranch:   baseBranch,
	})
	if err != nil {
		return fmt.Errorf("ensure worktree for phase %s: %w", spec.phase.String(), err)
	}

	stageInput := newWorkerStageInput(
		runner.repoRoot,
		worktreeResult.Path,
		task,
		roles.StageWork,
		spec.role,
		1,
		runner.cfg,
		func(msg string) {
			if msg == "" {
				return
			}
			fmt.Fprintf(runner.stderr, "Warning: %s\n", msg)
		},
	)

	stageResult, err := worker.StageEnvAndPrompts(stageInput)
	if err != nil {
		return fmt.Errorf("stage planning prompts: %w", err)
	}

	dispatchResult, err := worker.DispatchWorkerFromConfig(runner.cfg, task, stageResult, worktreeResult.Path, roles.StageWork, func(msg string) {
		if msg == "" {
			return
		}
		fmt.Fprintf(runner.stderr, "Warning: %s\n", msg)
	})
	if err != nil {
		return fmt.Errorf("dispatch phase %s agent: %w", spec.phase.String(), err)
	}

	record := state.RecordFor(spec.phase)
	record.Agent = phase.AgentMetadata{
		PID:       dispatchResult.PID,
		StartedAt: dispatchResult.StartedAt,
	}
	state.SetRecord(spec.phase, record)
	state.Notes = fmt.Sprintf("phase %d dispatched", spec.phase.Number())

	if err := runner.store.Save(*state); err != nil {
		return fmt.Errorf("save phase state: %w", err)
	}

	runner.emitPhaseDispatched(spec.phase, dispatchResult.PID)
	return nil
}

func (runner *phaseRunner) completePhase(state *phase.State) error {
	current := state.Current
	if state.LastCompleted >= current {
		return nil
	}
	next := current.Next()
	if err := runner.ensurePhasePrereqs(next); err != nil {
		return err
	}

	record := state.RecordFor(current)
	record.CompletedAt = runner.now()
	state.SetRecord(current, record)
	state.LastCompleted = current
	state.Current = next
	state.Notes = fmt.Sprintf("phase %d completed", state.LastCompleted.Number())

	if err := runner.store.Save(*state); err != nil {
		return fmt.Errorf("save phase state: %w", err)
	}

	runner.emitPhaseComplete(state.LastCompleted)
	return nil
}

func (runner *phaseRunner) ensurePhasePrereqs(target phase.Phase) error {
	validations, err := phase.ValidatePrerequisites(runner.repoRoot, target)
	if err != nil {
		return fmt.Errorf("validate phase %d prerequisites: %w", target.Number(), err)
	}

	failed := collectFailedValidations(validations)
	if len(failed) > 0 {
		for _, validation := range failed {
			fmt.Fprintf(runner.stderr, "phase gate: %s (%s)\n", validation.Name, validation.Message)
		}
		return fmt.Errorf("phase %d (%s) blocked by missing artifacts", target.Number(), target)
	}
	return nil
}

// collectPhaseCompletion finalizes the phase worktree and merges it into the base branch.
func (runner *phaseRunner) collectPhaseCompletion(spec phaseSpec) error {
	taskID := fmt.Sprintf("phase-%s", spec.phase.String())
	branchName := fmt.Sprintf("phase-%s", spec.phase.String())
	baseBranch := strings.TrimSpace(runner.cfg.Branches.Base)
	if baseBranch == "" {
		baseBranch = config.Defaults().Branches.Base
	}

	worktreePath, err := runner.worktreeManager.WorktreePath(taskID)
	if err != nil {
		return fmt.Errorf("resolve worktree for phase %s: %w", spec.phase.String(), err)
	}
	workerStateDir := workerStateDirPath(worktreePath, 1, roles.StageWork, spec.role)
	exitStatus, finished, err := worker.ReadExitStatus(workerStateDir, taskID, roles.StageWork)
	if err != nil {
		return fmt.Errorf("read phase exit status: %w", err)
	}
	if !finished {
		return fmt.Errorf("phase %d (%s) agent exited without exit.json", spec.phase.Number(), spec.phase)
	}
	if exitStatus.ExitCode != 0 {
		return fmt.Errorf("phase %d (%s) agent failed with exit code %d", spec.phase.Number(), spec.phase, exitStatus.ExitCode)
	}

	phaseTask := index.Task{
		ID:    taskID,
		Title: fmt.Sprintf("Phase %d %s", spec.phase.Number(), spec.phase.String()),
		Path:  spec.promptPath,
		Role:  spec.role,
	}
	if _, err := finalizeStageSuccess(worktreePath, workerStateDir, phaseTask, roles.StageWork); err != nil {
		return fmt.Errorf("finalize phase %s: %w", spec.phase.String(), err)
	}
	if err := runner.mergePlanningBranch(baseBranch, branchName); err != nil {
		return err
	}
	return nil
}

// mergePlanningBranch fast-forwards the base branch with the phase branch after validating cleanliness.
func (runner *phaseRunner) mergePlanningBranch(baseBranch string, phaseBranch string) error {
	if err := ensureCleanRepoRoot(runner.repoRoot); err != nil {
		return err
	}
	if err := runGitInRepo(runner.repoRoot, "checkout", baseBranch); err != nil {
		return fmt.Errorf("checkout base branch %s: %w", baseBranch, err)
	}
	if err := runGitInRepo(runner.repoRoot, "merge", "--ff-only", phaseBranch); err != nil {
		return fmt.Errorf("merge phase branch %s: %w", phaseBranch, err)
	}
	return nil
}

// ensureCleanRepoRoot verifies the repository root has no uncommitted changes (ignoring local-state).
func ensureCleanRepoRoot(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	cmd := exec.Command("git", "status", "--porcelain")
	cmd.Dir = repoRoot
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("check repo status: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if strings.HasPrefix(path, "_governator/_local-state") {
			continue
		}
		return fmt.Errorf("repository has uncommitted changes: %s", line)
	}
	return nil
}

func (runner *phaseRunner) isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	if err = proc.Signal(syscall.Signal(0)); err == nil {
		return true
	}
	return false
}

func (runner *phaseRunner) now() time.Time {
	return time.Now().UTC()
}

func (runner *phaseRunner) emitPhaseRunning(p phase.Phase, pid int) {
	fmt.Fprintf(runner.stdout, "phase %d running (pid %d)\n", p.Number(), pid)
}

func (runner *phaseRunner) emitPhaseDispatched(p phase.Phase, pid int) {
	fmt.Fprintf(runner.stdout, "phase %d dispatched (pid %d)\n", p.Number(), pid)
}

func (runner *phaseRunner) emitPhaseAgentComplete(p phase.Phase, pid int) {
	fmt.Fprintf(runner.stdout, "phase %d agent %d complete\n", p.Number(), pid)
}

func (runner *phaseRunner) emitPhaseComplete(p phase.Phase) {
	fmt.Fprintf(runner.stdout, "phase %d complete\n", p.Number())
}

func collectFailedValidations(validations []phase.ArtifactValidation) []phase.ArtifactValidation {
	var failed []phase.ArtifactValidation
	for _, validation := range validations {
		if !validation.Valid {
			failed = append(failed, validation)
		}
	}
	return failed
}

type phaseSpec struct {
	phase      phase.Phase
	promptPath string
	role       index.Role
}

var planningPhaseSpecs = map[phase.Phase]phaseSpec{
	phase.PhaseArchitectureBaseline: {
		phase:      phase.PhaseArchitectureBaseline,
		promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "architecture-baseline.md")),
		role:       index.Role("architect"),
	},
	phase.PhaseGapAnalysis: {
		phase:      phase.PhaseGapAnalysis,
		promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "gap-analysis.md")),
		role:       index.Role("default"),
	},
	phase.PhaseProjectPlanning: {
		phase:      phase.PhaseProjectPlanning,
		promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "roadmap.md")),
		role:       index.Role("planner"),
	},
	phase.PhaseTaskPlanning: {
		phase:      phase.PhaseTaskPlanning,
		promptPath: filepath.ToSlash(filepath.Join("_governator", "prompts", "task-planning.md")),
		role:       index.Role("planner"),
	},
}
