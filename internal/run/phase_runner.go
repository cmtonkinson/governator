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
	"github.com/cmtonkinson/governator/internal/inflight"
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
	worktreeManager     worktree.Manager
	worktreeManagerInit bool
	planning            planningTask
	inFlightStore       inflight.Store
	inFlight            inflight.Set
}

func newPhaseRunner(repoRoot string, cfg config.Config, opts Options, inFlightStore inflight.Store, inFlight inflight.Set, planning planningTask) *phaseRunner {
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return &phaseRunner{
		repoRoot:      repoRoot,
		cfg:           cfg,
		stdout:        stdout,
		stderr:        stderr,
		planning:      planning,
		inFlight:      inFlight,
		inFlightStore: inFlightStore,
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

func (runner *phaseRunner) logf(format string, args ...any) {
	if runner.stderr != nil {
		fmt.Fprintf(runner.stderr, format+"\n", args...)
	}
}

func (runner *phaseRunner) EnsurePlanningPhases(idx *index.Index) (bool, error) {
	if err := runner.ensureWorktreeManager(); err != nil {
		return false, fmt.Errorf("create worktree manager: %w", err)
	}
	if runner.inFlight == nil {
		runner.inFlight = inflight.Set{}
	}
	controller := newPlanningController(runner, idx)
	worker := newWorkstreamRunner()
	return worker.Run(controller)
}

// resolvePlanningPaths derives the worktree and worker state paths for a planning step.
func (runner *phaseRunner) resolvePlanningPaths(step workstreamStep, entry inflight.Entry) (string, string, error) {
	taskID := step.workstreamID()
	worktreePath := strings.TrimSpace(entry.Worktree)
	if worktreePath == "" {
		resolved, ok, err := runner.worktreeManager.ExistingWorktreePath(taskID)
		if err != nil {
			return "", "", fmt.Errorf("resolve worktree for step %s: %w", step.name, err)
		}
		if !ok {
			return "", "", fmt.Errorf("resolve worktree for step %s: missing worktree", step.name)
		}
		worktreePath = resolved
	}
	workerStateDir := strings.TrimSpace(entry.WorkerStateDir)
	if workerStateDir == "" {
		workerStateDir = workerStateDirPath(worktreePath, 1, roles.StageWork, step.role)
	}
	return worktreePath, workerStateDir, nil
}

// runningPlanningPID returns a live pid for the step when one can be observed.
func (runner *phaseRunner) runningPlanningPID(workerStateDir string, taskID string, stage roles.Stage) int {
	if _, finished, err := worker.ReadExitStatus(workerStateDir, taskID, stage); err == nil && finished {
		return 0
	}
	if pid, found, err := worker.ReadAgentPID(workerStateDir); err == nil && found && runner.isProcessAlive(pid) {
		return pid
	}
	return 0
}

// persistInFlight writes the in-flight set to durable local state.
func (runner *phaseRunner) persistInFlight() error {
	if runner.inFlight == nil {
		return nil
	}
	if err := runner.inFlightStore.Save(runner.inFlight); err != nil {
		return fmt.Errorf("save in-flight tasks: %w", err)
	}
	return nil
}

func (runner *phaseRunner) dispatchPhase(step workstreamStep) error {
	taskID := step.workstreamID()
	task := index.Task{
		ID:   taskID,
		Path: step.promptPath,
		Kind: index.TaskKindPlanning,
		Role: step.role,
	}

	phaseName := stepToPhase(step.name)

	branchName := step.branchName()
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
		return fmt.Errorf("ensure worktree for step %s: %w", step.name, err)
	}

	stageInput := newWorkerStageInput(
		runner.repoRoot,
		worktreeResult.Path,
		task,
		roles.StageWork,
		step.role,
		1,
		runner.cfg,
		func(msg string) {
			if msg == "" {
				return
			}
			fmt.Fprintf(runner.stderr, "Warning: %s\n", msg)
		},
	)
	stageInput.WorkerStateDir = planningWorkerStateDir(worktreeResult.Path, step)

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
		return fmt.Errorf("dispatch step %s agent: %w", step.name, err)
	}

	if err := runner.inFlight.AddWithStartAndPath(taskID, dispatchResult.StartedAt, worktreeResult.Path, dispatchResult.WorkerStateDir, string(roles.StageWork), string(step.role)); err != nil {
		return fmt.Errorf("record planning in-flight: %w", err)
	}
	if err := runner.persistInFlight(); err != nil {
		return err
	}

	runner.emitPhaseDispatched(phaseName, dispatchResult.PID)
	return nil
}

func planningWorkerStateDir(worktreePath string, step workstreamStep) string {
	dirName := planningStepWorkstreamID(step)
	if strings.TrimSpace(dirName) == "" {
		dirName = "planning"
	}
	return filepath.Join(worktreePath, localStateDirName, dirName)
}

func (runner *phaseRunner) completePhase(step workstreamStep) error {
	// New validation engine doesn't use phase-based gating
	// Validations are run after worker completion in the controller

	phaseName := stepToPhase(step.name)
	runner.emitPhaseComplete(phaseName)
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

// ensureStepGate evaluates the configured gate target, defaulting when none is specified.
func (runner *phaseRunner) ensureStepGate(target workstreamGateTarget, defaultPhase phase.Phase) error {
	if target.enabled {
		return runner.ensurePhasePrereqs(target.phase)
	}
	return runner.ensurePhasePrereqs(defaultPhase)
}

// collectPhaseCompletion finalizes the phase worktree and merges it into the base branch.
func (runner *phaseRunner) collectPhaseCompletion(step workstreamStep, worktreePath string, workerStateDir string) error {
	taskID := step.workstreamID()
	branchName := step.branchName()
	baseBranch := strings.TrimSpace(runner.cfg.Branches.Base)
	if baseBranch == "" {
		baseBranch = config.Defaults().Branches.Base
	}
	exitStatus, finished, err := worker.ReadExitStatus(workerStateDir, taskID, roles.StageWork)
	if err != nil {
		return fmt.Errorf("read phase exit status: %w", err)
	}
	if !finished {
		return fmt.Errorf("step %s agent exited without exit.json", step.name)
	}
	if exitStatus.ExitCode != 0 {
		return fmt.Errorf("step %s agent failed with exit code %d", step.name, exitStatus.ExitCode)
	}

	phaseTask := index.Task{
		ID:    taskID,
		Title: step.title(),
		Path:  step.promptPath,
		Kind:  index.TaskKindPlanning,
		Role:  step.role,
	}
	if _, err := finalizeStageSuccess(worktreePath, workerStateDir, phaseTask, roles.StageWork); err != nil {
		return fmt.Errorf("finalize step %s: %w", step.name, err)
	}
	if err := UpdatePlanningIndex(worktreePath, step); err != nil {
		return fmt.Errorf("update planning index: %w", err)
	}
	if step.actions.mergeToBase {
		if err := runner.mergePlanningBranch(baseBranch, branchName, step.title()); err != nil {
			return err
		}
	}
	return nil
}

// mergePlanningBranch fast-forwards the base branch with the phase branch after validating cleanliness.
func (runner *phaseRunner) mergePlanningBranch(baseBranch string, phaseBranch string, stepTitle string) error {
	if err := ensureCleanRepoRoot(runner.repoRoot); err != nil {
		return err
	}
	if err := runGitInRepo(runner.repoRoot, "checkout", baseBranch); err != nil {
		return fmt.Errorf("checkout base branch %s: %w", baseBranch, err)
	}
	if err := commitPlanningIndexIfDirty(runner.repoRoot, stepTitle); err != nil {
		return err
	}
	if err := runGitInRepo(runner.repoRoot, "merge", "--no-ff", "--no-edit", phaseBranch); err != nil {
		return fmt.Errorf("merge phase branch %s: %w", phaseBranch, err)
	}
	return nil
}

// commitPlanningIndexIfDirty commits the planning index on the base branch when modified.
func commitPlanningIndexIfDirty(repoRoot string, stepTitle string) error {
	status, err := runGitOutput(repoRoot, "status", "--porcelain", "--", indexFilePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if err := runGit(repoRoot, "add", "--", indexFilePath); err != nil {
		return err
	}
	subject := strings.TrimSpace(stepTitle)
	if subject == "" {
		subject = "planning"
	}
	message := fmt.Sprintf("[planning] %s index", subject)
	return runGitWithEnv(repoRoot, []string{
		"GIT_AUTHOR_NAME=Governator CLI",
		"GIT_AUTHOR_EMAIL=governator@localhost",
		"GIT_COMMITTER_NAME=Governator CLI",
		"GIT_COMMITTER_EMAIL=governator@localhost",
	}, "commit", "-m", message)
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
	trimmedOutput := strings.TrimRight(string(output), "\n")
	for _, line := range strings.Split(trimmedOutput, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if len(line) < 3 {
			return fmt.Errorf("repository has uncommitted changes: %s", line)
		}
		path := strings.TrimSpace(line[3:])
		if strings.HasPrefix(path, "_governator/_local-state") {
			continue
		}
		if strings.HasPrefix(path, "_governator/index.json") {
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
