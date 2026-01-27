// Package run provides the main orchestration logic for the run command.
package run

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/scheduler"
	"github.com/cmtonkinson/governator/internal/worker"
	"github.com/cmtonkinson/governator/internal/worktree"
)

const (
	// indexFilePath is the relative path to the task index file.
	indexFilePath = "_governator/task-index.json"
)

// Options defines the configuration for a run execution.
type Options struct {
	Stdout io.Writer
	Stderr io.Writer
}

// Result captures the outcome of a run execution.
type Result struct {
	ResumedTasks []string
	BlockedTasks []string
	Message      string
}

// Run executes the main run orchestration including resume logic and task execution.
func Run(repoRoot string, opts Options) (Result, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return Result{}, fmt.Errorf("repo root is required")
	}
	if opts.Stdout == nil || opts.Stderr == nil {
		return Result{}, fmt.Errorf("stdout and stderr are required")
	}

	// Load configuration
	cfg, err := config.Load(repoRoot, nil, nil)
	if err != nil {
		return Result{}, fmt.Errorf("load config: %w", err)
	}

	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		return Result{}, fmt.Errorf("create in-flight store: %w", err)
	}
	inFlight, err := inFlightStore.Load()
	if err != nil {
		return Result{}, fmt.Errorf("load in-flight tasks: %w", err)
	}
	if inFlight == nil {
		inFlight = inflight.Set{}
	}

	// Load task index
	indexPath := filepath.Join(repoRoot, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		return Result{}, fmt.Errorf("load task index: %w", err)
	}

	planning := newPlanningTask()
	complete, err := planningComplete(idx, planning)
	if err != nil {
		return Result{}, fmt.Errorf("planning index: %w", err)
	}
	if !complete {
		phaseRunner := newPhaseRunner(repoRoot, cfg, opts, inFlightStore, inFlight)
		handled, err := phaseRunner.EnsurePlanningPhases(&idx)
		if err != nil {
			return Result{}, fmt.Errorf("run planning: %w", err)
		}
		if handled {
			return Result{}, nil
		}
		return Result{}, fmt.Errorf("planning tasks still pending")
	}

	// Build role caps before executing stages
	caps := scheduler.RoleCapsFromConfig(cfg)
	baseBranch := baseBranchName(cfg)

	// Check for planning drift
	if err := CheckPlanningDrift(repoRoot, idx.Digests); err != nil {
		if errors.Is(err, ErrPlanningDrift) {
			emitPlanningDriftMessage(opts.Stdout, err.Error())
		}
		return Result{}, err
	}

	// Set up audit logging
	auditor, err := audit.NewLogger(repoRoot, opts.Stderr)
	if err != nil {
		return Result{}, fmt.Errorf("create audit logger: %w", err)
	}

	var guard *SelfRunGuard
	if cfg.AutoRerun.Enabled {
		guard = newSelfRunGuard(repoRoot, cfg.AutoRerun, auditor)
		guardOutcome, err := guard.EnsureAllowed()
		if err != nil {
			return Result{}, fmt.Errorf("run guard: %w", err)
		}
		if !guardOutcome.Allowed {
			return Result{Message: guardOutcome.Message}, nil
		}
		defer func() {
			if releaseErr := guard.Release(); releaseErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to release run guard: %v\n", releaseErr)
			}
		}()
	}

	// Detect resume candidates
	candidates, err := DetectResumeCandidates(repoRoot, idx, cfg)
	if err != nil {
		return Result{}, fmt.Errorf("detect resume candidates: %w", err)
	}

	// Process resume candidates
	resumeResult := ProcessResumeCandidates(candidates, cfg)

	var resumedTasks []string
	var blockedTasks []string

	// Process tasks to be resumed
	for _, candidate := range resumeResult.Resumed {
		if err := PrepareTaskForResume(&idx, candidate.Task.ID, auditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to prepare task %s for resume: %v\n", candidate.Task.ID, err)
			continue
		}
		resumedTasks = append(resumedTasks, candidate.Task.ID)
		fmt.Fprintf(opts.Stdout, "Resuming task %s (attempt %d)\n", candidate.Task.ID, candidate.Task.Attempts.Total+1)
	}

	// Process tasks that exceeded retry limits
	for _, candidate := range resumeResult.Blocked {
		maxAttempts := getMaxAttempts(candidate.Task, cfg)
		if err := BlockTaskWithRetryExceeded(&idx, candidate.Task.ID, maxAttempts, auditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to block task %s: %v\n", candidate.Task.ID, err)
			continue
		}
		blockedTasks = append(blockedTasks, candidate.Task.ID)
		fmt.Fprintf(opts.Stdout, "Task %s blocked: exceeded retry limit (%d attempts)\n", candidate.Task.ID, maxAttempts)
	}

	// Execute task stages via the workstream runner.
	resumeWorktrees := resumeWorktreeMap(resumeResult.Resumed)
	executionController := newExecutionController(repoRoot, &idx, cfg, caps, inFlight, resumeWorktrees, auditor, auditor, opts, baseBranch)
	executionRunner := newWorkstreamRunner()
	if _, err := executionRunner.Run(executionController); err != nil {
		return Result{}, fmt.Errorf("execute execution stages: %w", err)
	}
	workResult := executionController.workResult
	testResult := executionController.testResult
	reviewResult := executionController.reviewResult
	conflictResult := executionController.conflictResult
	mergeResult := executionController.mergeResult
	branchResult := executionController.branchResult

	// Save updated index
	if len(resumedTasks) > 0 || len(blockedTasks) > 0 || workResult.TasksWorked > 0 || workResult.TasksBlocked > 0 || testResult.TasksTested > 0 || testResult.TasksBlocked > 0 || reviewResult.TasksReviewed > 0 || reviewResult.TasksBlocked > 0 || conflictResult.TasksResolved > 0 || conflictResult.TasksBlocked > 0 || mergeResult.TasksProcessed > 0 || branchResult.BranchesCreated > 0 {
		if err := index.Save(indexPath, idx); err != nil {
			return Result{}, fmt.Errorf("save task index: %w", err)
		}
	}
	if executionController.inFlightWasUpdated {
		if err := inFlightStore.Save(inFlight); err != nil {
			return Result{}, fmt.Errorf("save in-flight tasks: %w", err)
		}
	}

	// Build result message
	var message strings.Builder
	if len(resumedTasks) > 0 {
		message.WriteString(fmt.Sprintf("Resumed %d task(s)", len(resumedTasks)))
	}
	if len(blockedTasks) > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		message.WriteString(fmt.Sprintf("blocked %d task(s) due to retry limit", len(blockedTasks)))
	}
	if workResult.TasksDispatched > 0 || workResult.TasksWorked > 0 || workResult.TasksBlocked > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		if workResult.TasksDispatched > 0 {
			message.WriteString(fmt.Sprintf("dispatched %d work task(s)", workResult.TasksDispatched))
		} else {
			message.WriteString(fmt.Sprintf("collected %d work task(s)", workResult.TasksWorked+workResult.TasksBlocked))
		}
	}
	if testResult.TasksDispatched > 0 || testResult.TasksTested > 0 || testResult.TasksBlocked > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		if testResult.TasksDispatched > 0 {
			message.WriteString(fmt.Sprintf("dispatched %d test task(s)", testResult.TasksDispatched))
		} else {
			message.WriteString(fmt.Sprintf("collected %d test task(s)", testResult.TasksTested+testResult.TasksBlocked))
		}
	}
	if reviewResult.TasksDispatched > 0 || reviewResult.TasksReviewed > 0 || reviewResult.TasksBlocked > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		if reviewResult.TasksDispatched > 0 {
			message.WriteString(fmt.Sprintf("dispatched %d review task(s)", reviewResult.TasksDispatched))
		} else {
			message.WriteString(fmt.Sprintf("collected %d review task(s)", reviewResult.TasksReviewed+reviewResult.TasksBlocked))
		}
	}
	if conflictResult.TasksDispatched > 0 || conflictResult.TasksResolved > 0 || conflictResult.TasksBlocked > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		if conflictResult.TasksDispatched > 0 {
			message.WriteString(fmt.Sprintf("dispatched %d conflict resolution task(s)", conflictResult.TasksDispatched))
		} else {
			message.WriteString(fmt.Sprintf("collected %d conflict resolution task(s)", conflictResult.TasksResolved+conflictResult.TasksBlocked))
		}
	}
	if mergeResult.TasksProcessed > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		message.WriteString(fmt.Sprintf("processed %d merge task(s)", mergeResult.TasksProcessed))
	}
	if branchResult.BranchesCreated > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		message.WriteString(fmt.Sprintf("created %d branch(es) for open tasks", branchResult.BranchesCreated))
	}
	if message.Len() == 0 {
		message.WriteString("No tasks to resume or execute")
	}

	return Result{
		ResumedTasks: resumedTasks,
		BlockedTasks: blockedTasks,
		Message:      message.String(),
	}, nil
}

// TestStageResult captures the outcome of test stage execution.
type TestStageResult struct {
	TasksDispatched int
	TasksTested     int
	TasksBlocked    int
	InFlightUpdated bool
}

// WorkStageResult captures the outcome of work stage execution.
type WorkStageResult struct {
	TasksDispatched int
	TasksWorked     int
	TasksBlocked    int
	InFlightUpdated bool
	WorktreePaths   map[string]string
}

// ExecuteWorkStage processes tasks in the open state through the work stage.
func ExecuteWorkStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, inFlight inflight.Set, resumeWorktrees map[string]string, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (WorkStageResult, error) {
	result := WorkStageResult{
		WorktreePaths: map[string]string{},
	}
	if inFlight == nil {
		inFlight = inflight.Set{}
	}
	if resumeWorktrees == nil {
		resumeWorktrees = map[string]string{}
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	baseBranch := baseBranchName(cfg)

	for _, task := range idx.Tasks {
		if task.State != index.TaskStateTriaged || !inFlight.Contains(task.ID) {
			continue
		}
		worktreePath, ok := worktreePathForTask(inFlight, task.ID)
		if !ok {
			worktreePath, err = resolveWorktreePath(manager, task, resumeWorktrees)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
				continue
			}
		}

		entry, entryOK := inFlight.Entry(task.ID)
		if !entryOK || strings.TrimSpace(entry.WorkerStateDir) == "" {
			fmt.Fprintf(opts.Stderr, "Warning: missing worker state dir for task %s\n", task.ID)
			continue
		}

		exitStatus, finished, err := worker.ReadExitStatus(entry.WorkerStateDir, task.ID, roles.StageWork)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to read exit status for task %s: %v\n", task.ID, err)
			continue
		}
		if !finished {
			if startedAt, ok := startedAtForTask(inFlight, task.ID); ok && timedOut(startedAt, cfg.Timeouts.WorkerSeconds) {
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: formatTimeoutReason(cfg.Timeouts.WorkerSeconds),
					TimedOut:    true,
				}
				logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageWork, statusFromIngestResult(failedResult), exitCodeForOutcome(-1, true), func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
				}
				if updateErr := UpdateTaskStateFromWorkResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				} else {
					result.TasksBlocked++
					emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork), failedResult.BlockReason, cfg.Timeouts.WorkerSeconds)
				}
				if workerAuditor != nil {
					if auditErr := workerAuditor.LogWorkerTimeout(task.ID, string(task.Role), cfg.Timeouts.WorkerSeconds, worktreePath); auditErr != nil {
						fmt.Fprintf(opts.Stderr, "Warning: failed to log worker timeout for %s: %v\n", task.ID, auditErr)
					}
				}
				killWorkerProcess(task.PID, entry.WorkerStateDir, func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := inFlight.Remove(task.ID); err == nil {
					result.InFlightUpdated = true
				}
			}
			continue
		}

		var ingestResult worker.IngestResult
		if exitStatus.ExitCode != 0 {
			ingestResult = worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("worker process exited with code %d", exitStatus.ExitCode),
			}
		} else {
			ingestResult, err = finalizeStageSuccess(worktreePath, entry.WorkerStateDir, task, roles.StageWork)
			if err != nil {
				ingestResult = worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("governator git finalize failed: %v", err),
				}
			}
		}

		logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageWork, statusFromIngestResult(ingestResult), exitCodeForOutcome(exitStatus.ExitCode, ingestResult.TimedOut), func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})

		if err := UpdateTaskStateFromWorkResult(idx, task.ID, ingestResult, transitionAuditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
			continue
		}
		if !ingestResult.Success {
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
		}

		if ingestResult.Success {
			result.TasksWorked++
			result.WorktreePaths[task.ID] = worktreePath
			emitTaskComplete(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork))
		} else if ingestResult.TimedOut {
			result.TasksBlocked++
			emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork), ingestResult.BlockReason, cfg.Timeouts.WorkerSeconds)
		} else {
			result.TasksBlocked++
			emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork), ingestResult.BlockReason)
		}

		if err := inFlight.Remove(task.ID); err == nil {
			result.InFlightUpdated = true
		}
	}

	adjustedCaps := adjustCapsForInFlight(caps, *idx, inFlight)
	selectedTasks, err := selectTasksForStage(*idx, adjustedCaps, inFlight, index.TaskStateTriaged)
	if err != nil {
		return result, fmt.Errorf("schedule work tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	for _, task := range selectedTasks {
		attempt, err := ensureWorkAttempt(idx, task.ID)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to set attempt for task %s: %v\n", task.ID, err)
			continue
		}

		worktreePath := ""
		branchName := TaskBranchName(task)
		if resumePath, ok := resumeWorktrees[task.ID]; ok && strings.TrimSpace(resumePath) != "" {
			if err := validateWorktreePath(resumePath); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to validate resume worktree for task %s: %v\n", task.ID, err)
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("resume worktree invalid: %v", err),
				}
				if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
				}
				if updateErr := UpdateTaskStateFromWorkResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				} else {
					result.TasksBlocked++
					emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork), failedResult.BlockReason)
				}
				continue
			}
			worktreePath = resumePath
		} else {
			spec := worktree.Spec{
				WorkstreamID: task.ID,
				Branch:       branchName,
				BaseBranch:   baseBranch,
			}
			worktreeResult, err := manager.EnsureWorktree(spec)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to ensure worktree for task %s: %v\n", task.ID, err)
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("worktree setup failed: %v", err),
				}
				if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
				}
				if updateErr := UpdateTaskStateFromWorkResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				} else {
					result.TasksBlocked++
					emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork), failedResult.BlockReason)
				}
				continue
			}
			worktreePath = worktreeResult.Path

			if !worktreeResult.Reused && workerAuditor != nil {
				if err := workerAuditor.LogWorktreeCreate(task.ID, string(task.Role), worktreeResult.RelativePath, branchName); err != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to log worktree create for task %s: %v\n", task.ID, err)
				}
			}
		}

		emitTaskStart(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork))

		stageInput := newWorkerStageInput(
			repoRoot,
			worktreePath,
			task,
			roles.StageWork,
			task.Role,
			attempt,
			cfg,
			func(msg string) {
				fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
			},
		)
		stageResult, err := worker.StageEnvAndPrompts(stageInput)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to stage work environment for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("work agent staging failed: %v", err),
			}
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
			if updateErr := UpdateTaskStateFromWorkResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork), failedResult.BlockReason)
			}
			continue
		}

		dispatchResult, err := worker.DispatchWorkerFromConfig(cfg, task, stageResult, worktreePath, roles.StageWork, func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		})
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to dispatch work agent for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("work agent dispatch failed: %v", err),
			}
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
			if updateErr := UpdateTaskStateFromWorkResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageWork), failedResult.BlockReason)
			}
			continue
		}

		logAgentInvoke(workerAuditor, task.ID, task.Role, roles.StageWork, attempt, func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})
		if err := recordTaskDispatch(idx, task.ID, dispatchResult.PID, string(task.Role)); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to record dispatch metadata for task %s: %v\n", task.ID, err)
		}

		if err := inFlight.AddWithStartAndPath(task.ID, dispatchResult.StartedAt, worktreePath, dispatchResult.WorkerStateDir, string(roles.StageWork), string(task.Role)); err == nil {
			result.InFlightUpdated = true
		}
		result.TasksDispatched++
		result.WorktreePaths[task.ID] = worktreePath
	}

	return result, nil
}

// ExecuteWorkAgent runs the work agent for a specific task.
func ExecuteWorkAgent(repoRoot, worktreePath string, task index.Task, cfg config.Config, auditor *audit.Logger, opts Options) (worker.IngestResult, error) {
	stageInput := newWorkerStageInput(
		repoRoot,
		worktreePath,
		task,
		roles.StageWork,
		task.Role,
		maxInt(task.Attempts.Total, 1),
		cfg,
		func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	)

	logAgentInvoke(auditor, task.ID, task.Role, roles.StageWork, maxInt(task.Attempts.Total, 1), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	stageResult, err := worker.StageEnvAndPrompts(stageInput)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("stage work environment: %w", err)
	}

	execResult, err := worker.ExecuteWorkerFromConfigWithAudit(cfg, task, stageResult, worktreePath, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	}, auditor, worktreePath)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("execute work worker: %w", err)
	}

	var ingestResult worker.IngestResult
	if execResult.Error != nil {
		blockReason := fmt.Sprintf("worker execution failed: %s", execResult.Error.Error())
		if execResult.TimedOut {
			blockReason = fmt.Sprintf("worker execution timed out after %s", execResult.Duration)
		}
		ingestResult = worker.IngestResult{
			Success:     false,
			NewState:    index.TaskStateBlocked,
			BlockReason: blockReason,
			TimedOut:    execResult.TimedOut,
		}
	} else {
		ingestResult, err = finalizeStageSuccess(worktreePath, stageResult.WorkerStateDir, task, roles.StageWork)
		if err != nil {
			return worker.IngestResult{}, fmt.Errorf("finalize work result: %w", err)
		}
	}

	logAgentOutcome(auditor, task.ID, task.Role, roles.StageWork, statusFromIngestResult(ingestResult), exitCodeForOutcome(execResult.ExitCode, execResult.TimedOut), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	return ingestResult, nil
}

// UpdateTaskStateFromWorkResult updates the task index based on work execution results.
func UpdateTaskStateFromWorkResult(idx *index.Index, taskID string, workResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateBlocked
	if workResult.Success {
		target = workResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
	}
	if err := updateIndexTask(idx, taskID, func(task *index.Task) {
		task.PID = 0
		if workResult.Success {
			task.BlockedReason = ""
			task.MergeConflict = false
		} else {
			task.BlockedReason = workResult.BlockReason
		}
	}); err != nil {
		return fmt.Errorf("task %q metadata: %w", taskID, err)
	}
	return nil
}

// ExecuteTestStage processes tasks in the worked state through the test stage.
func ExecuteTestStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, inFlight inflight.Set, worktreeOverrides map[string]string, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (TestStageResult, error) {
	result := TestStageResult{}
	if inFlight == nil {
		inFlight = inflight.Set{}
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	for _, task := range idx.Tasks {
		if task.State != index.TaskStateImplemented || !inFlight.Contains(task.ID) {
			continue
		}
		worktreePath, ok := worktreePathForTask(inFlight, task.ID)
		if !ok {
			worktreePath, err = resolveWorktreePath(manager, task, worktreeOverrides)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
				continue
			}
		}
		entry, entryOK := inFlight.Entry(task.ID)
		if !entryOK || strings.TrimSpace(entry.WorkerStateDir) == "" {
			fmt.Fprintf(opts.Stderr, "Warning: missing worker state dir for task %s\n", task.ID)
			continue
		}

		exitStatus, finished, err := worker.ReadExitStatus(entry.WorkerStateDir, task.ID, roles.StageTest)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to read exit status for task %s: %v\n", task.ID, err)
			continue
		}
		if !finished {
			if startedAt, ok := startedAtForTask(inFlight, task.ID); ok && timedOut(startedAt, cfg.Timeouts.WorkerSeconds) {
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: formatTimeoutReason(cfg.Timeouts.WorkerSeconds),
					TimedOut:    true,
				}
				logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageTest, statusFromIngestResult(failedResult), exitCodeForOutcome(-1, true), func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
				}
				if updateErr := UpdateTaskStateFromTestResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				} else {
					result.TasksBlocked++
					emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), failedResult.BlockReason, cfg.Timeouts.WorkerSeconds)
				}
				if workerAuditor != nil {
					if auditErr := workerAuditor.LogWorkerTimeout(task.ID, string(task.Role), cfg.Timeouts.WorkerSeconds, worktreePath); auditErr != nil {
						fmt.Fprintf(opts.Stderr, "Warning: failed to log worker timeout for %s: %v\n", task.ID, auditErr)
					}
				}
				killWorkerProcess(task.PID, entry.WorkerStateDir, func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := inFlight.Remove(task.ID); err == nil {
					result.InFlightUpdated = true
				}
			}
			continue
		}

		var ingestResult worker.IngestResult
		if exitStatus.ExitCode != 0 {
			ingestResult = worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("worker process exited with code %d", exitStatus.ExitCode),
			}
		} else {
			ingestResult, err = finalizeStageSuccess(worktreePath, entry.WorkerStateDir, task, roles.StageTest)
			if err != nil {
				ingestResult = worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("governator git finalize failed: %v", err),
				}
			}
		}

		logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageTest, statusFromIngestResult(ingestResult), exitCodeForOutcome(exitStatus.ExitCode, ingestResult.TimedOut), func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})

		if err := UpdateTaskStateFromTestResult(idx, task.ID, ingestResult, transitionAuditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
			continue
		}
		if !ingestResult.Success {
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
		}

		if ingestResult.Success {
			result.TasksTested++
			emitTaskComplete(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest))
		} else if ingestResult.TimedOut {
			result.TasksBlocked++
			emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), ingestResult.BlockReason, cfg.Timeouts.WorkerSeconds)
		} else {
			result.TasksBlocked++
			emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), ingestResult.BlockReason)
		}

		if err := inFlight.Remove(task.ID); err == nil {
			result.InFlightUpdated = true
		}
	}

	adjustedCaps := adjustCapsForInFlight(caps, *idx, inFlight)
	selectedTasks, err := selectTasksForStage(*idx, adjustedCaps, inFlight, index.TaskStateImplemented)
	if err != nil {
		return result, fmt.Errorf("schedule test tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	for _, task := range selectedTasks {
		worktreePath, ok := worktreePathForTask(inFlight, task.ID)
		if !ok {
			worktreePath, err = resolveWorktreePath(manager, task, worktreeOverrides)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
				continue
			}
		}

		emitTaskStart(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest))

		stageInput := newWorkerStageInput(
			repoRoot,
			worktreePath,
			task,
			roles.StageTest,
			task.Role,
			maxInt(task.Attempts.Total, 1),
			cfg,
			func(msg string) {
				fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
			},
		)

		stageResult, err := worker.StageEnvAndPrompts(stageInput)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to stage test environment for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("test agent staging failed: %v", err),
			}
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
			if updateErr := UpdateTaskStateFromTestResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), failedResult.BlockReason)
			}
			continue
		}

		dispatchResult, err := worker.DispatchWorkerFromConfig(cfg, task, stageResult, worktreePath, roles.StageTest, func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		})
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to dispatch test agent for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("test agent dispatch failed: %v", err),
			}
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
			if updateErr := UpdateTaskStateFromTestResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), failedResult.BlockReason)
			}
			continue
		}

		logAgentInvoke(workerAuditor, task.ID, task.Role, roles.StageTest, maxInt(task.Attempts.Total, 1), func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})
		if err := recordTaskDispatch(idx, task.ID, dispatchResult.PID, string(task.Role)); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to record dispatch metadata for task %s: %v\n", task.ID, err)
		}

		if err := inFlight.AddWithStartAndPath(task.ID, dispatchResult.StartedAt, worktreePath, dispatchResult.WorkerStateDir, string(roles.StageTest), string(task.Role)); err == nil {
			result.InFlightUpdated = true
		}
		result.TasksDispatched++
	}

	return result, nil
}

// ExecuteTestAgent runs the test agent for a specific task.
func ExecuteTestAgent(repoRoot, worktreePath string, task index.Task, cfg config.Config, auditor *audit.Logger, opts Options) (worker.IngestResult, error) {
	// Stage environment and prompts for test execution
	stageInput := newWorkerStageInput(
		repoRoot,
		worktreePath,
		task,
		roles.StageTest,
		task.Role,
		maxInt(task.Attempts.Total, 1),
		cfg,
		func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	)

	logAgentInvoke(auditor, task.ID, task.Role, roles.StageTest, maxInt(task.Attempts.Total, 1), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	stageResult, err := worker.StageEnvAndPrompts(stageInput)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("stage test environment: %w", err)
	}

	// Execute the test worker
	execResult, err := worker.ExecuteWorkerFromConfigWithAudit(cfg, task, stageResult, worktreePath, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	}, auditor, worktreePath)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("execute test worker: %w", err)
	}

	var ingestResult worker.IngestResult
	if execResult.Error != nil {
		blockReason := fmt.Sprintf("worker execution failed: %s", execResult.Error.Error())
		if execResult.TimedOut {
			blockReason = fmt.Sprintf("worker execution timed out after %s", execResult.Duration)
		}
		ingestResult = worker.IngestResult{
			Success:     false,
			NewState:    index.TaskStateBlocked,
			BlockReason: blockReason,
			TimedOut:    execResult.TimedOut,
		}
	} else {
		ingestResult, err = finalizeStageSuccess(worktreePath, stageResult.WorkerStateDir, task, roles.StageTest)
		if err != nil {
			return worker.IngestResult{}, fmt.Errorf("finalize test result: %w", err)
		}
	}

	logAgentOutcome(auditor, task.ID, task.Role, roles.StageTest, statusFromIngestResult(ingestResult), exitCodeForOutcome(execResult.ExitCode, execResult.TimedOut), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	return ingestResult, nil
}

// UpdateTaskStateFromTestResult updates the task index based on test execution results.
func UpdateTaskStateFromTestResult(idx *index.Index, taskID string, testResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateBlocked
	if testResult.Success {
		target = testResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
	}
	if err := updateIndexTask(idx, taskID, func(task *index.Task) {
		task.PID = 0
		if testResult.Success {
			task.BlockedReason = ""
			task.MergeConflict = false
		} else {
			task.BlockedReason = testResult.BlockReason
		}
	}); err != nil {
		return fmt.Errorf("task %q metadata: %w", taskID, err)
	}
	return nil
}

// ReviewStageResult captures the outcome of review stage execution.
type ReviewStageResult struct {
	TasksDispatched int
	TasksReviewed   int
	TasksBlocked    int
	InFlightUpdated bool
}

// ExecuteReviewStage processes tasks in the tested state through the review stage.
func ExecuteReviewStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, inFlight inflight.Set, worktreeOverrides map[string]string, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (ReviewStageResult, error) {
	result := ReviewStageResult{}
	if inFlight == nil {
		inFlight = inflight.Set{}
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	for _, task := range idx.Tasks {
		if task.State != index.TaskStateTested || !inFlight.Contains(task.ID) {
			continue
		}
		worktreePath, ok := worktreePathForTask(inFlight, task.ID)
		if !ok {
			worktreePath, err = resolveWorktreePath(manager, task, worktreeOverrides)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
				continue
			}
		}
		entry, entryOK := inFlight.Entry(task.ID)
		if !entryOK || strings.TrimSpace(entry.WorkerStateDir) == "" {
			fmt.Fprintf(opts.Stderr, "Warning: missing worker state dir for task %s\n", task.ID)
			continue
		}

		exitStatus, finished, err := worker.ReadExitStatus(entry.WorkerStateDir, task.ID, roles.StageReview)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to read exit status for task %s: %v\n", task.ID, err)
			continue
		}
		if !finished {
			if startedAt, ok := startedAtForTask(inFlight, task.ID); ok && timedOut(startedAt, cfg.Timeouts.WorkerSeconds) {
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateTriaged,
					BlockReason: formatTimeoutReason(cfg.Timeouts.WorkerSeconds),
					TimedOut:    true,
				}
				logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageReview, statusFromIngestResult(failedResult), exitCodeForOutcome(-1, true), func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := UpdateTaskStateFromReviewResult(idx, task.ID, failedResult, transitionAuditor); err != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
				} else {
					result.TasksBlocked++
					emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), failedResult.BlockReason, cfg.Timeouts.WorkerSeconds)
				}
				if workerAuditor != nil {
					if auditErr := workerAuditor.LogWorkerTimeout(task.ID, string(task.Role), cfg.Timeouts.WorkerSeconds, worktreePath); auditErr != nil {
						fmt.Fprintf(opts.Stderr, "Warning: failed to log worker timeout for %s: %v\n", task.ID, auditErr)
					}
				}
				killWorkerProcess(task.PID, entry.WorkerStateDir, func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := inFlight.Remove(task.ID); err == nil {
					result.InFlightUpdated = true
				}
			}
			continue
		}

		var reviewResult worker.IngestResult
		if exitStatus.ExitCode != 0 {
			reviewResult = worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateTriaged,
				BlockReason: fmt.Sprintf("worker process exited with code %d", exitStatus.ExitCode),
			}
		} else {
			reviewResult, err = finalizeStageSuccess(worktreePath, entry.WorkerStateDir, task, roles.StageReview)
			if err != nil {
				reviewResult = worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateTriaged,
					BlockReason: fmt.Sprintf("governator git finalize failed: %v", err),
				}
			}
		}

		logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageReview, statusFromIngestResult(reviewResult), exitCodeForOutcome(exitStatus.ExitCode, reviewResult.TimedOut), func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})

		if reviewResult.Success {
			if err := UpdateTaskStateFromReviewResult(idx, task.ID, reviewResult, transitionAuditor); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s after review: %v\n", task.ID, err)
				continue
			}
			result.TasksReviewed++
			emitTaskComplete(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview))

			emitTaskStart(opts.Stdout, task.ID, string(task.Role), mergeStageName)
			if err := applyTaskStateTransition(idx, task.ID, index.TaskStateMergeable, transitionAuditor); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to mark %s as mergeable: %v\n", task.ID, err)
			}
			mergeInput := MergeFlowInput{
				RepoRoot:     repoRoot,
				WorktreePath: worktreePath,
				Task:         task,
				MainBranch:   baseBranchName(cfg),
				Auditor:      workerAuditor,
			}

			mergeResult, err := ExecuteReviewMergeFlow(mergeInput)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to execute merge flow for task %s: %v\n", task.ID, err)
				// TODO(cmtonkinson): Consider routing non-conflict merge failures to conflict instead of blocked.
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("merge flow failed: %v", err),
				}
				if updateErr := applyTaskStateTransition(idx, task.ID, index.TaskStateBlocked, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				} else {
					result.TasksBlocked++
					emitTaskFailure(opts.Stdout, task.ID, string(task.Role), mergeStageName, failedResult.BlockReason)
				}
				if err := inFlight.Remove(task.ID); err == nil {
					result.InFlightUpdated = true
				}
				continue
			}

			finalResult := worker.IngestResult{
				Success:     mergeResult.Success,
				NewState:    mergeResult.NewState,
				BlockReason: mergeResult.ConflictError,
			}
			if updateErr := UpdateTaskStateFromMerge(idx, task.ID, finalResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			}

			if mergeResult.Success {
				emitTaskComplete(opts.Stdout, task.ID, string(task.Role), mergeStageName)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), mergeStageName, mergeResult.ConflictError)
			}
		} else {
			if err := UpdateTaskStateFromReviewResult(idx, task.ID, reviewResult, transitionAuditor); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
			} else {
				result.TasksBlocked++
				if reviewResult.TimedOut {
					emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), reviewResult.BlockReason, cfg.Timeouts.WorkerSeconds)
				} else {
					emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), reviewResult.BlockReason)
				}
			}
		}

		if err := inFlight.Remove(task.ID); err == nil {
			result.InFlightUpdated = true
		}
	}

	adjustedCaps := adjustCapsForInFlight(caps, *idx, inFlight)
	selectedTasks, err := selectTasksForStage(*idx, adjustedCaps, inFlight, index.TaskStateTested)
	if err != nil {
		return result, fmt.Errorf("schedule review tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	for _, task := range selectedTasks {
		worktreePath, err := resolveWorktreePath(manager, task, worktreeOverrides)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

		emitTaskStart(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview))

		stageInput := newWorkerStageInput(
			repoRoot,
			worktreePath,
			task,
			roles.StageReview,
			task.Role,
			maxInt(task.Attempts.Total, 1),
			cfg,
			func(msg string) {
				fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
			},
		)

		stageResult, err := worker.StageEnvAndPrompts(stageInput)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to stage review environment for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateTriaged,
				BlockReason: fmt.Sprintf("review agent staging failed: %v", err),
			}
			if updateErr := UpdateTaskStateFromReviewResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), failedResult.BlockReason)
			}
			continue
		}

		dispatchResult, err := worker.DispatchWorkerFromConfig(cfg, task, stageResult, worktreePath, roles.StageReview, func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		})
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to dispatch review agent for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateTriaged,
				BlockReason: fmt.Sprintf("review agent dispatch failed: %v", err),
			}
			if updateErr := UpdateTaskStateFromReviewResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), failedResult.BlockReason)
			}
			continue
		}

		logAgentInvoke(workerAuditor, task.ID, task.Role, roles.StageReview, maxInt(task.Attempts.Total, 1), func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})
		if err := recordTaskDispatch(idx, task.ID, dispatchResult.PID, string(task.Role)); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to record dispatch metadata for task %s: %v\n", task.ID, err)
		}

		if err := inFlight.AddWithStartAndPath(task.ID, dispatchResult.StartedAt, worktreePath, dispatchResult.WorkerStateDir, string(roles.StageReview), string(task.Role)); err == nil {
			result.InFlightUpdated = true
		}
		result.TasksDispatched++
	}

	return result, nil
}

// ExecuteReviewAgent runs the review agent for a specific task.
func ExecuteReviewAgent(repoRoot, worktreePath string, task index.Task, cfg config.Config, auditor *audit.Logger, opts Options) (worker.IngestResult, error) {
	// Stage environment and prompts for review execution
	stageInput := newWorkerStageInput(
		repoRoot,
		worktreePath,
		task,
		roles.StageReview,
		task.Role,
		maxInt(task.Attempts.Total, 1),
		cfg,
		func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	)

	logAgentInvoke(auditor, task.ID, task.Role, roles.StageReview, maxInt(task.Attempts.Total, 1), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	stageResult, err := worker.StageEnvAndPrompts(stageInput)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("stage review environment: %w", err)
	}

	// Execute the review worker
	execResult, err := worker.ExecuteWorkerFromConfigWithAudit(cfg, task, stageResult, worktreePath, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	}, auditor, worktreePath)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("execute review worker: %w", err)
	}

	var ingestResult worker.IngestResult
	if execResult.Error != nil {
		blockReason := fmt.Sprintf("worker execution failed: %s", execResult.Error.Error())
		if execResult.TimedOut {
			blockReason = fmt.Sprintf("worker execution timed out after %s", execResult.Duration)
		}
		ingestResult = worker.IngestResult{
			Success:     false,
			NewState:    index.TaskStateTriaged,
			BlockReason: blockReason,
			TimedOut:    execResult.TimedOut,
		}
	} else {
		ingestResult, err = finalizeStageSuccess(worktreePath, stageResult.WorkerStateDir, task, roles.StageReview)
		if err != nil {
			return worker.IngestResult{}, fmt.Errorf("finalize review result: %w", err)
		}
	}

	logAgentOutcome(auditor, task.ID, task.Role, roles.StageReview, statusFromIngestResult(ingestResult), exitCodeForOutcome(execResult.ExitCode, execResult.TimedOut), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	return ingestResult, nil
}

// UpdateTaskStateFromReviewResult updates the task index based on review execution results.
func UpdateTaskStateFromReviewResult(idx *index.Index, taskID string, reviewResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateTriaged
	if reviewResult.Success {
		target = reviewResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
	}
	if err := updateIndexTask(idx, taskID, func(task *index.Task) {
		task.PID = 0
		if reviewResult.Success {
			task.BlockedReason = ""
			task.MergeConflict = false
		} else {
			task.BlockedReason = reviewResult.BlockReason
		}
	}); err != nil {
		return fmt.Errorf("task %q metadata: %w", taskID, err)
	}
	return nil
}

// ConflictResolutionStageResult captures the outcome of conflict resolution stage execution.
type ConflictResolutionStageResult struct {
	TasksDispatched int
	TasksResolved   int
	TasksBlocked    int
	InFlightUpdated bool
}

// ExecuteConflictResolutionStage processes tasks in the conflict state by dispatching conflict resolution agents.
func ExecuteConflictResolutionStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, inFlight inflight.Set, worktreeOverrides map[string]string, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (ConflictResolutionStageResult, error) {
	result := ConflictResolutionStageResult{}
	if inFlight == nil {
		inFlight = inflight.Set{}
	}

	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	for _, task := range idx.Tasks {
		if task.State != index.TaskStateConflict || !inFlight.Contains(task.ID) {
			continue
		}
		worktreePath, err := resolveWorktreePath(manager, task, worktreeOverrides)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}
		entry, entryOK := inFlight.Entry(task.ID)
		if !entryOK || strings.TrimSpace(entry.WorkerStateDir) == "" {
			fmt.Fprintf(opts.Stderr, "Warning: missing worker state dir for task %s\n", task.ID)
			continue
		}

		exitStatus, finished, err := worker.ReadExitStatus(entry.WorkerStateDir, task.ID, roles.StageResolve)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to read exit status for task %s: %v\n", task.ID, err)
			continue
		}
		if !finished {
			if startedAt, ok := startedAtForTask(inFlight, task.ID); ok && timedOut(startedAt, cfg.Timeouts.WorkerSeconds) {
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: formatTimeoutReason(cfg.Timeouts.WorkerSeconds),
					TimedOut:    true,
				}
				logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageResolve, statusFromIngestResult(failedResult), exitCodeForOutcome(-1, true), func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
				}
				if updateErr := UpdateTaskStateFromConflictResolution(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				} else {
					result.TasksBlocked++
					emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageResolve), failedResult.BlockReason, cfg.Timeouts.WorkerSeconds)
				}
				if workerAuditor != nil {
					if auditErr := workerAuditor.LogWorkerTimeout(task.ID, string(task.Role), cfg.Timeouts.WorkerSeconds, worktreePath); auditErr != nil {
						fmt.Fprintf(opts.Stderr, "Warning: failed to log worker timeout for %s: %v\n", task.ID, auditErr)
					}
				}
				killWorkerProcess(task.PID, entry.WorkerStateDir, func(message string) {
					fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
				})
				if err := inFlight.Remove(task.ID); err == nil {
					result.InFlightUpdated = true
				}
			}
			continue
		}

		var ingestResult worker.IngestResult
		if exitStatus.ExitCode != 0 {
			ingestResult = worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("worker process exited with code %d", exitStatus.ExitCode),
			}
		} else {
			ingestResult, err = finalizeStageSuccess(worktreePath, entry.WorkerStateDir, task, roles.StageResolve)
			if err != nil {
				ingestResult = worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("governator git finalize failed: %v", err),
				}
			}
		}

		logAgentOutcome(workerAuditor, task.ID, task.Role, roles.StageResolve, statusFromIngestResult(ingestResult), exitCodeForOutcome(exitStatus.ExitCode, ingestResult.TimedOut), func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})

		if err := UpdateTaskStateFromConflictResolution(idx, task.ID, ingestResult, transitionAuditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
			continue
		}
		if !ingestResult.Success {
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
		}

		if ingestResult.Success {
			result.TasksResolved++
			emitTaskComplete(opts.Stdout, task.ID, string(task.Role), string(roles.StageResolve))
		} else if ingestResult.TimedOut {
			result.TasksBlocked++
			emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageResolve), ingestResult.BlockReason, cfg.Timeouts.WorkerSeconds)
		} else {
			result.TasksBlocked++
			emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageResolve), ingestResult.BlockReason)
		}

		if err := inFlight.Remove(task.ID); err == nil {
			result.InFlightUpdated = true
		}
	}

	adjustedCaps := adjustCapsForInFlight(caps, *idx, inFlight)
	selectedTasks, err := selectTasksForStage(*idx, adjustedCaps, inFlight, index.TaskStateConflict)
	if err != nil {
		return result, fmt.Errorf("schedule conflict resolution tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	for _, task := range selectedTasks {
		worktreePath, err := resolveWorktreePath(manager, task, worktreeOverrides)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

		roleResult, err := SelectRoleForConflictResolution(repoRoot, task, cfg, *idx, workerAuditor, opts)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to select conflict resolution role for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("conflict resolution role selection failed: %v", err),
			}
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
			if updateErr := UpdateTaskStateFromConflictResolution(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, resolveRoleForLogs(roleResult.Role, task.Role), string(roles.StageResolve), failedResult.BlockReason)
			}
			continue
		}

		roleForLogs := resolveRoleForLogs(roleResult.Role, task.Role)
		emitTaskStart(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve))

		stageInput := newWorkerStageInput(
			repoRoot,
			worktreePath,
			task,
			roles.StageResolve,
			roleResult.Role,
			maxInt(task.Attempts.Total, 1),
			cfg,
			func(msg string) {
				fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
			},
		)
		stageResult, err := worker.StageEnvAndPrompts(stageInput)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to stage resolve environment for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("conflict resolution staging failed: %v", err),
			}
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
			if updateErr := UpdateTaskStateFromConflictResolution(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve), failedResult.BlockReason)
			}
			continue
		}

		dispatchResult, err := worker.DispatchWorkerFromConfig(cfg, task, stageResult, worktreePath, roles.StageResolve, func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		})
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to dispatch conflict resolution agent for task %s: %v\n", task.ID, err)
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("conflict resolution dispatch failed: %v", err),
			}
			if err := index.IncrementTaskFailedAttempt(idx, task.ID); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to increment failed attempts for %s: %v\n", task.ID, err)
			}
			if updateErr := UpdateTaskStateFromConflictResolution(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve), failedResult.BlockReason)
			}
			continue
		}

		logAgentInvoke(workerAuditor, task.ID, roleResult.Role, roles.StageResolve, maxInt(task.Attempts.Total, 1), func(message string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
		})

		if err := recordTaskDispatch(idx, task.ID, dispatchResult.PID, string(roleResult.Role)); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to record dispatch metadata for task %s: %v\n", task.ID, err)
		}
		if err := inFlight.AddWithStartAndPath(task.ID, dispatchResult.StartedAt, worktreePath, dispatchResult.WorkerStateDir, string(roles.StageResolve), string(roleResult.Role)); err == nil {
			result.InFlightUpdated = true
		}
		result.TasksDispatched++
	}

	return result, nil
}

// ExecuteConflictResolutionAgent runs the conflict resolution agent for a specific task.
func ExecuteConflictResolutionAgent(repoRoot, worktreePath string, task index.Task, cfg config.Config, idx index.Index, auditor *audit.Logger, opts Options) (worker.IngestResult, roles.RoleAssignmentResult, error) {
	// Use role assignment to select appropriate role for conflict resolution
	roleResult, err := SelectRoleForConflictResolution(repoRoot, task, cfg, idx, auditor, opts)
	if err != nil {
		return worker.IngestResult{}, roleResult, fmt.Errorf("select role for conflict resolution: %w", err)
	}

	// Stage environment and prompts for conflict resolution execution
	stageInput := newWorkerStageInput(
		repoRoot,
		worktreePath,
		task,
		roles.StageResolve,
		roleResult.Role,
		maxInt(task.Attempts.Total, 1),
		cfg,
		func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	)

	stageResult, err := worker.StageEnvAndPrompts(stageInput)
	if err != nil {
		return worker.IngestResult{}, roleResult, fmt.Errorf("stage conflict resolution environment: %w", err)
	}

	roleForLogs := resolveRoleForLogs(roleResult.Role, task.Role)
	emitTaskStart(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve))

	logAgentInvoke(auditor, task.ID, roleResult.Role, roles.StageResolve, maxInt(task.Attempts.Total, 1), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	// Execute the conflict resolution worker
	execResult, err := worker.ExecuteWorkerFromConfigWithAudit(cfg, task, stageResult, worktreePath, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	}, auditor, worktreePath)
	if err != nil {
		return worker.IngestResult{}, roleResult, fmt.Errorf("execute conflict resolution worker: %w", err)
	}

	var ingestResult worker.IngestResult
	if execResult.Error != nil {
		blockReason := fmt.Sprintf("worker execution failed: %s", execResult.Error.Error())
		if execResult.TimedOut {
			blockReason = fmt.Sprintf("worker execution timed out after %s", execResult.Duration)
		}
		ingestResult = worker.IngestResult{
			Success:     false,
			NewState:    index.TaskStateBlocked,
			BlockReason: blockReason,
			TimedOut:    execResult.TimedOut,
		}
	} else {
		ingestResult, err = finalizeStageSuccess(worktreePath, stageResult.WorkerStateDir, task, roles.StageResolve)
		if err != nil {
			return worker.IngestResult{}, roleResult, fmt.Errorf("finalize conflict resolution result: %w", err)
		}
	}

	logAgentOutcome(auditor, task.ID, roleResult.Role, roles.StageResolve, statusFromIngestResult(ingestResult), exitCodeForOutcome(execResult.ExitCode, execResult.TimedOut), func(message string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", message)
	})

	return ingestResult, roleResult, nil
}

// UpdateTaskStateFromConflictResolution updates the task index based on conflict resolution results.
func UpdateTaskStateFromConflictResolution(idx *index.Index, taskID string, resolutionResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateBlocked
	if resolutionResult.Success {
		target = resolutionResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
	}
	if err := updateIndexTask(idx, taskID, func(task *index.Task) {
		task.PID = 0
		if resolutionResult.Success {
			task.BlockedReason = ""
			task.MergeConflict = false
		} else {
			task.BlockedReason = resolutionResult.BlockReason
		}
	}); err != nil {
		return fmt.Errorf("task %q metadata: %w", taskID, err)
	}
	return nil
}

// SelectRoleForConflictResolution uses the role assignment LLM to select an appropriate role for conflict resolution.
func SelectRoleForConflictResolution(repoRoot string, task index.Task, cfg config.Config, idx index.Index, auditor *audit.Logger, opts Options) (roles.RoleAssignmentResult, error) {
	// Load role assignment prompt
	promptTemplate, err := roles.LoadRoleAssignmentPrompt(repoRoot)
	if err != nil {
		return roles.RoleAssignmentResult{}, fmt.Errorf("load role assignment prompt: %w", err)
	}

	// Load role registry to get available roles
	registry, err := roles.LoadRegistry(repoRoot, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	})
	if err != nil {
		return roles.RoleAssignmentResult{}, fmt.Errorf("load role registry: %w", err)
	}

	availableRoles := registry.Roles()
	if len(availableRoles) == 0 {
		return roles.RoleAssignmentResult{}, fmt.Errorf("no roles available for conflict resolution")
	}

	// Read task content for role assignment
	taskContent, err := os.ReadFile(filepath.Join(repoRoot, task.Path))
	if err != nil {
		return roles.RoleAssignmentResult{}, fmt.Errorf("read task file %s: %w", task.Path, err)
	}

	// Build role assignment request
	request := roles.RoleAssignmentRequest{
		Task: roles.RoleAssignmentTask{
			ID:      task.ID,
			Title:   task.Title,
			Path:    task.Path,
			Content: string(taskContent),
		},
		Stage:          roles.StageResolve,
		AvailableRoles: availableRoles,
		Caps: roles.RoleAssignmentCaps{
			Global:      cfg.Concurrency.Global,
			DefaultRole: cfg.Concurrency.DefaultRole,
			Roles:       make(map[index.Role]int),
			InFlight:    buildRoleInFlightCounts(idx),
		},
	}

	// Copy role-specific caps
	for role, cap := range cfg.Concurrency.Roles {
		request.Caps.Roles[index.Role(role)] = cap
	}

	invoker, err := newWorkerCommandInvoker(cfg, repoRoot, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	})
	if err != nil {
		return roles.RoleAssignmentResult{}, fmt.Errorf("create role assignment invoker: %w", err)
	}

	result, err := roles.SelectRole(
		context.Background(),
		invoker,
		promptTemplate,
		request,
		func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
		auditor,
	)
	if err != nil {
		return roles.RoleAssignmentResult{}, fmt.Errorf("select role for conflict resolution: %w", err)
	}

	return result, nil
}

// resolveRoleForLogs selects the best role string for logging purposes.
func resolveRoleForLogs(primary index.Role, fallback index.Role) string {
	if role := strings.TrimSpace(string(primary)); role != "" {
		return role
	}
	if role := strings.TrimSpace(string(fallback)); role != "" {
		return role
	}
	return ""
}

func baseBranchName(cfg config.Config) string {
	branch := strings.TrimSpace(cfg.Branches.Base)
	if branch != "" {
		return branch
	}
	return config.Defaults().Branches.Base
}

func buildRoleInFlightCounts(idx index.Index) map[index.Role]int {
	counts := map[index.Role]int{}
	for _, task := range idx.Tasks {
		if !isRoleInFlight(task.State) {
			continue
		}
		role := strings.TrimSpace(string(task.Role))
		if role == "" {
			continue
		}
		counts[task.Role]++
	}
	return counts
}

// resumeWorktreeMap builds a lookup of task id to preserved worktree paths.
func resumeWorktreeMap(candidates []ResumeCandidate) map[string]string {
	if len(candidates) == 0 {
		return map[string]string{}
	}
	paths := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Task.ID) == "" || strings.TrimSpace(candidate.WorktreePath) == "" {
			continue
		}
		paths[candidate.Task.ID] = candidate.WorktreePath
	}
	return paths
}

// mergeWorktreeOverrides combines multiple worktree override maps.
func mergeWorktreeOverrides(primary map[string]string, secondary map[string]string) map[string]string {
	merged := map[string]string{}
	for key, value := range primary {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		merged[key] = value
	}
	for key, value := range secondary {
		if strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
			continue
		}
		merged[key] = value
	}
	return merged
}

// ensureWorkAttempt increments attempts for a task starting work and returns the current attempt.
func ensureWorkAttempt(idx *index.Index, taskID string) (int, error) {
	if idx == nil {
		return 0, fmt.Errorf("index is nil")
	}
	task, err := findIndexTask(idx, taskID)
	if err != nil {
		return 0, err
	}
	if task.Attempts.Total <= 0 {
		if err := index.IncrementTaskAttempt(idx, taskID); err != nil {
			return 0, err
		}
	}
	task, err = findIndexTask(idx, taskID)
	if err != nil {
		return 0, err
	}
	return task.Attempts.Total, nil
}

// resolveWorktreePath returns the worktree path for the task, honoring overrides.
func resolveWorktreePath(manager worktree.Manager, task index.Task, overrides map[string]string) (string, error) {
	if overrides != nil {
		if path, ok := overrides[task.ID]; ok && strings.TrimSpace(path) != "" {
			return path, nil
		}
	}
	if path, ok, err := manager.ExistingWorktreePath(task.ID); err != nil {
		return "", err
	} else if ok {
		return path, nil
	}
	return manager.WorktreePath(task.ID)
}

// validateWorktreePath ensures the worktree path exists and is a directory.
func validateWorktreePath(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("worktree path is required")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat worktree path %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("worktree path %s is not a directory", path)
	}
	return nil
}

func isRoleInFlight(state index.TaskState) bool {
	switch state {
	case index.TaskStateImplemented, index.TaskStateTested, index.TaskStateConflict, index.TaskStateResolved:
		return true
	default:
		return false
	}
}

// MergeStageResult captures the outcome of merge stage execution.
type MergeStageResult struct {
	TasksProcessed int
	TasksMerged    int
	TasksConflict  int
}

// ExecuteMergeStage processes tasks in the resolved state through the merge flow.
func ExecuteMergeStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, worktreeOverrides map[string]string, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (MergeStageResult, error) {
	result := MergeStageResult{}

	selectedTasks, err := selectTasksForStage(*idx, caps, nil, index.TaskStateResolved)
	if err != nil {
		return result, fmt.Errorf("schedule merge tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	// Set up worktree manager
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	// Process each resolved task through merge flow
	for _, task := range selectedTasks {
		result.TasksProcessed++

		// Get the worktree path for the task
		worktreePath, err := resolveWorktreePath(manager, task, worktreeOverrides)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

		if err := applyTaskStateTransition(idx, task.ID, index.TaskStateMergeable, transitionAuditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to mark %s as mergeable before merge stage: %v\n", task.ID, err)
		}
		emitTaskStart(opts.Stdout, task.ID, string(task.Role), mergeStageName)

		// Execute conflict resolution merge flow
		mergeInput := MergeFlowInput{
			RepoRoot:     repoRoot,
			WorktreePath: worktreePath,
			Task:         task,
			MainBranch:   baseBranchName(cfg),
			Auditor:      workerAuditor,
		}

		mergeResult, err := ExecuteConflictResolutionMergeFlow(mergeInput)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to execute merge flow for task %s: %v\n", task.ID, err)
			// Create a blocked result for merge flow failure
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("merge flow failed: %v", err),
			}
			if updateErr := UpdateTaskStateFromMerge(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				continue
			}
			emitTaskFailure(opts.Stdout, task.ID, string(task.Role), mergeStageName, failedResult.BlockReason)
			continue
		}

		// Update task state based on merge result
		finalResult := worker.IngestResult{
			Success:     mergeResult.Success,
			NewState:    mergeResult.NewState,
			BlockReason: mergeResult.ConflictError,
		}

		if updateErr := UpdateTaskStateFromMerge(idx, task.ID, finalResult, transitionAuditor); updateErr != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			continue
		}

		if mergeResult.Success {
			result.TasksMerged++
			emitTaskComplete(opts.Stdout, task.ID, string(task.Role), mergeStageName)
		} else {
			result.TasksConflict++
			emitTaskFailure(opts.Stdout, task.ID, string(task.Role), mergeStageName, mergeResult.ConflictError)
		}
	}

	return result, nil
}

func selectTasksForStage(idx index.Index, caps scheduler.RoleCaps, inFlight inflight.Set, states ...index.TaskState) ([]index.Task, error) {
	if len(states) == 0 {
		return nil, nil
	}
	ordered, err := scheduler.OrderedEligibleTasks(idx, inFlightMap(inFlight))
	if err != nil {
		return nil, err
	}
	stateSet := make(map[index.TaskState]struct{}, len(states))
	for _, state := range states {
		stateSet[state] = struct{}{}
	}
	filtered := make([]index.Task, 0, len(ordered))
	for _, task := range ordered {
		if _, ok := stateSet[task.State]; ok {
			filtered = append(filtered, task)
		}
	}
	if len(filtered) == 0 {
		return nil, nil
	}
	result := scheduler.RouteOrderedTasks(filtered, caps)
	return result.Selected, nil
}

func applyTaskStateTransition(idx *index.Index, taskID string, target index.TaskState, auditor index.TransitionAuditor) error {
	if idx == nil {
		return fmt.Errorf("index is nil")
	}
	task, err := findIndexTask(idx, taskID)
	if err != nil {
		return err
	}
	if task.State == target {
		return nil
	}
	if err := index.TransitionTaskStateWithAudit(idx, taskID, target, auditor); err != nil {
		return err
	}
	return nil
}

func findIndexTask(idx *index.Index, taskID string) (*index.Task, error) {
	if idx == nil {
		return nil, fmt.Errorf("index is nil")
	}
	if strings.TrimSpace(taskID) == "" {
		return nil, fmt.Errorf("task id is required")
	}
	for i := range idx.Tasks {
		if idx.Tasks[i].ID == taskID {
			return &idx.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("task %q not found in index", taskID)
}

func updateIndexTask(idx *index.Index, taskID string, updater func(*index.Task)) error {
	if idx == nil {
		return fmt.Errorf("index is nil")
	}
	if strings.TrimSpace(taskID) == "" {
		return fmt.Errorf("task id is required")
	}
	for i := range idx.Tasks {
		if idx.Tasks[i].ID == taskID {
			updater(&idx.Tasks[i])
			return nil
		}
	}
	return fmt.Errorf("task %q not found in index", taskID)
}

func recordTaskDispatch(idx *index.Index, taskID string, pid int, assignedRole string) error {
	return updateIndexTask(idx, taskID, func(task *index.Task) {
		task.PID = pid
		task.AssignedRole = assignedRole
		task.BlockedReason = ""
	})
}

// UpdateTaskStateFromMerge updates the task index based on merge results.
func UpdateTaskStateFromMerge(idx *index.Index, taskID string, mergeResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateConflict
	if mergeResult.Success {
		target = mergeResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
	}
	if err := updateIndexTask(idx, taskID, func(task *index.Task) {
		task.PID = 0
		task.BlockedReason = mergeResult.BlockReason
		task.MergeConflict = target == index.TaskStateConflict
	}); err != nil {
		return fmt.Errorf("task %q metadata: %w", taskID, err)
	}
	return nil
}

// BranchStageResult captures the outcome of branch creation for open tasks.
type BranchStageResult struct {
	BranchesCreated int
	BranchesSkipped int
}

// EnsureBranchesForOpenTasks creates branches for tasks in the open state.
func EnsureBranchesForOpenTasks(repoRoot string, idx *index.Index, auditor *audit.Logger, opts Options, baseBranch string) (BranchStageResult, error) {
	result := BranchStageResult{}

	// Find tasks in open state
	var openTasks []index.Task
	for _, task := range idx.Tasks {
		if task.Kind != index.TaskKindExecution {
			continue
		}
		if task.State == index.TaskStateTriaged {
			openTasks = append(openTasks, task)
		}
	}

	if len(openTasks) == 0 {
		return result, nil
	}

	effectiveBranch := strings.TrimSpace(baseBranch)
	if effectiveBranch == "" {
		effectiveBranch = config.Defaults().Branches.Base
	}

	// Create branch lifecycle manager
	branchManager := NewBranchLifecycleManager(repoRoot, auditor)

	// Process each open task to ensure it has a branch
	for _, task := range openTasks {
		// Check if branch already exists
		branchName := branchManager.GetTaskBranchName(task)
		exists, err := branchManager.BranchExists(branchName)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to check if branch exists for task %s: %v\n", task.ID, err)
			continue
		}

		if exists {
			result.BranchesSkipped++
			continue
		}

		// Create branch for the task
		if err := branchManager.CreateTaskBranch(task, effectiveBranch); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to create branch for task %s: %v\n", task.ID, err)
			continue
		}

		result.BranchesCreated++
		fmt.Fprintf(opts.Stdout, "Created branch %s for task %s\n", branchName, task.ID)
	}

	return result, nil
}
