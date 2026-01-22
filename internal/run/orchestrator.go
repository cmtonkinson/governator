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

	// Build role caps before executing stages
	caps := scheduler.RoleCapsFromConfig(cfg)
	baseBranch := baseBranchName(cfg)

	// Load task index
	indexPath := filepath.Join(repoRoot, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		return Result{}, fmt.Errorf("load task index: %w", err)
	}

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

	// Execute test stage for worked tasks
	testResult, err := ExecuteTestStage(repoRoot, &idx, cfg, caps, auditor, auditor, opts)
	if err != nil {
		return Result{}, fmt.Errorf("execute test stage: %w", err)
	}

	// Execute review stage for tested tasks
	reviewResult, err := ExecuteReviewStage(repoRoot, &idx, cfg, caps, auditor, auditor, opts)
	if err != nil {
		return Result{}, fmt.Errorf("execute review stage: %w", err)
	}

	// Execute conflict resolution stage for conflict tasks
	conflictResult, err := ExecuteConflictResolutionStage(repoRoot, &idx, cfg, caps, auditor, auditor, opts)
	if err != nil {
		return Result{}, fmt.Errorf("execute conflict resolution stage: %w", err)
	}

	// Execute merge stage for resolved tasks
	mergeResult, err := ExecuteMergeStage(repoRoot, &idx, cfg, caps, auditor, auditor, opts)
	if err != nil {
		return Result{}, fmt.Errorf("execute merge stage: %w", err)
	}

	// Ensure branches exist for open tasks
	branchResult, err := EnsureBranchesForOpenTasks(repoRoot, &idx, auditor, opts, baseBranch)
	if err != nil {
		return Result{}, fmt.Errorf("ensure branches for open tasks: %w", err)
	}

	// Save updated index
	if len(resumedTasks) > 0 || len(blockedTasks) > 0 || testResult.TasksProcessed > 0 || reviewResult.TasksProcessed > 0 || conflictResult.TasksProcessed > 0 || mergeResult.TasksProcessed > 0 || branchResult.BranchesCreated > 0 {
		if err := index.Save(indexPath, idx); err != nil {
			return Result{}, fmt.Errorf("save task index: %w", err)
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
	if testResult.TasksProcessed > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		message.WriteString(fmt.Sprintf("processed %d test task(s)", testResult.TasksProcessed))
	}
	if reviewResult.TasksProcessed > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		message.WriteString(fmt.Sprintf("processed %d review task(s)", reviewResult.TasksProcessed))
	}
	if conflictResult.TasksProcessed > 0 {
		if message.Len() > 0 {
			message.WriteString(", ")
		}
		message.WriteString(fmt.Sprintf("processed %d conflict resolution task(s)", conflictResult.TasksProcessed))
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
	TasksProcessed int
	TasksTested    int
	TasksBlocked   int
}

// ExecuteTestStage processes tasks in the worked state through the test stage.
func ExecuteTestStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (TestStageResult, error) {
	result := TestStageResult{}

	selectedTasks, err := selectTasksForStage(*idx, caps, index.TaskStateWorked)
	if err != nil {
		return result, fmt.Errorf("schedule test tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	// Set up worktree manager
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	// Process each worked task through test stage
	for _, task := range selectedTasks {
		result.TasksProcessed++

		// Get the worktree path for the task
		worktreePath, err := manager.WorktreePath(task.ID, task.Attempts.Total)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

		emitTaskStart(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest))

		// Execute test agent for the task
		testResult, err := ExecuteTestAgent(repoRoot, worktreePath, task, cfg, workerAuditor, opts)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to execute test agent for task %s: %v\n", task.ID, err)
			// Create a failed test result to update task state
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("test agent execution failed: %v", err),
			}
			if updateErr := UpdateTaskStateFromTestResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), failedResult.BlockReason)
			}
			continue
		}

		// Update task state based on test result
		if err := UpdateTaskStateFromTestResult(idx, task.ID, testResult, transitionAuditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
			continue
		}

		if testResult.Success {
			result.TasksTested++
			emitTaskComplete(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest))
		} else {
			result.TasksBlocked++
			if testResult.TimedOut {
				emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), testResult.BlockReason, cfg.Timeouts.WorkerSeconds)
			} else {
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageTest), testResult.BlockReason)
			}
		}
	}

	return result, nil
}

// ExecuteTestAgent runs the test agent for a specific task.
func ExecuteTestAgent(repoRoot, worktreePath string, task index.Task, cfg config.Config, auditor *audit.Logger, opts Options) (worker.IngestResult, error) {
	// Stage environment and prompts for test execution
	stageInput := worker.StageInput{
		RepoRoot:     repoRoot,
		WorktreeRoot: worktreePath,
		Task:         task,
		Stage:        roles.StageTest,
		Role:         task.Role, // Use task's assigned role for test stage
		Warn: func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	}

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

	// Ingest the worker result
	ingestInput := worker.IngestInput{
		TaskID:       task.ID,
		WorktreePath: worktreePath,
		Stage:        roles.StageTest,
		ExecResult:   execResult,
		Warn: func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	}

	ingestResult, err := worker.IngestWorkerResult(ingestInput)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("ingest test result: %w", err)
	}

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
	return nil
}

// ReviewStageResult captures the outcome of review stage execution.
type ReviewStageResult struct {
	TasksProcessed int
	TasksReviewed  int
	TasksBlocked   int
}

// ExecuteReviewStage processes tasks in the tested state through the review stage.
func ExecuteReviewStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (ReviewStageResult, error) {
	result := ReviewStageResult{}

	selectedTasks, err := selectTasksForStage(*idx, caps, index.TaskStateTested)
	if err != nil {
		return result, fmt.Errorf("schedule review tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	// Set up worktree manager
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	// Process each tested task through review stage
	for _, task := range selectedTasks {
		result.TasksProcessed++

		// Get the worktree path for the task
		worktreePath, err := manager.WorktreePath(task.ID, task.Attempts.Total)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

		emitTaskStart(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview))

		// Execute review agent for the task
		reviewResult, err := ExecuteReviewAgent(repoRoot, worktreePath, task, cfg, workerAuditor, opts)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to execute review agent for task %s: %v\n", task.ID, err)
			// Create a failed review result to update task state
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("review agent execution failed: %v", err),
			}
			if updateErr := UpdateTaskStateFromReviewResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), failedResult.BlockReason)
			}
			continue
		}

		// Update task state based on review result
		if err := UpdateTaskStateFromReviewResult(idx, task.ID, reviewResult, transitionAuditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
			continue
		}

		if reviewResult.Success {
			result.TasksReviewed++
			emitTaskComplete(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview))

			// Execute merge flow after review success
			emitTaskStart(opts.Stdout, task.ID, string(task.Role), mergeStageName)
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
				// Create a failed merge result to update task state
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("merge flow failed: %v", err),
				}
				if updateErr := UpdateTaskStateFromReviewResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
					continue
				}
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), mergeStageName, failedResult.BlockReason)
				continue
			}

			// Update task state based on merge result
			finalResult := worker.IngestResult{
				Success:     mergeResult.Success,
				NewState:    mergeResult.NewState,
				BlockReason: mergeResult.ConflictError,
			}

			if updateErr := UpdateTaskStateFromReviewResult(idx, task.ID, finalResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				continue
			}

			if mergeResult.Success {
				emitTaskComplete(opts.Stdout, task.ID, string(task.Role), mergeStageName)
			} else {
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), mergeStageName, mergeResult.ConflictError)
			}
		} else {
			result.TasksBlocked++
			if reviewResult.TimedOut {
				emitTaskTimeout(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), reviewResult.BlockReason, cfg.Timeouts.WorkerSeconds)
			} else {
				emitTaskFailure(opts.Stdout, task.ID, string(task.Role), string(roles.StageReview), reviewResult.BlockReason)
			}
		}
	}

	return result, nil
}

// ExecuteReviewAgent runs the review agent for a specific task.
func ExecuteReviewAgent(repoRoot, worktreePath string, task index.Task, cfg config.Config, auditor *audit.Logger, opts Options) (worker.IngestResult, error) {
	// Stage environment and prompts for review execution
	stageInput := worker.StageInput{
		RepoRoot:     repoRoot,
		WorktreeRoot: worktreePath,
		Task:         task,
		Stage:        roles.StageReview,
		Role:         task.Role, // Use task's assigned role for review stage
		Warn: func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	}

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

	// Ingest the worker result
	ingestInput := worker.IngestInput{
		TaskID:       task.ID,
		WorktreePath: worktreePath,
		Stage:        roles.StageReview,
		ExecResult:   execResult,
		Warn: func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	}

	ingestResult, err := worker.IngestWorkerResult(ingestInput)
	if err != nil {
		return worker.IngestResult{}, fmt.Errorf("ingest review result: %w", err)
	}

	return ingestResult, nil
}

// UpdateTaskStateFromReviewResult updates the task index based on review execution results.
func UpdateTaskStateFromReviewResult(idx *index.Index, taskID string, reviewResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateBlocked
	if reviewResult.Success {
		target = reviewResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
	}
	return nil
}

// ConflictResolutionStageResult captures the outcome of conflict resolution stage execution.
type ConflictResolutionStageResult struct {
	TasksProcessed int
	TasksResolved  int
	TasksBlocked   int
}

// ExecuteConflictResolutionStage processes tasks in the conflict state by dispatching conflict resolution agents.
func ExecuteConflictResolutionStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (ConflictResolutionStageResult, error) {
	result := ConflictResolutionStageResult{}

	selectedTasks, err := selectTasksForStage(*idx, caps, index.TaskStateConflict)
	if err != nil {
		return result, fmt.Errorf("schedule conflict resolution tasks: %w", err)
	}

	if len(selectedTasks) == 0 {
		return result, nil
	}

	// Set up worktree manager
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	// Process each conflict task through conflict resolution
	for _, task := range selectedTasks {
		result.TasksProcessed++

		// Get the worktree path for the task
		worktreePath, err := manager.WorktreePath(task.ID, task.Attempts.Total)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

		// Execute conflict resolution agent for the task
		resolutionResult, roleResult, err := ExecuteConflictResolutionAgent(repoRoot, worktreePath, task, cfg, *idx, workerAuditor, opts)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to execute conflict resolution agent for task %s: %v\n", task.ID, err)
			// Create a failed resolution result to update task state
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("conflict resolution agent execution failed: %v", err),
			}
			if updateErr := UpdateTaskStateFromConflictResolution(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				emitTaskFailure(opts.Stdout, task.ID, resolveRoleForLogs(roleResult.Role, task.Role), string(roles.StageResolve), failedResult.BlockReason)
			}
			continue
		}

		// Update task state based on resolution result
		if err := UpdateTaskStateFromConflictResolution(idx, task.ID, resolutionResult, transitionAuditor); err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
			continue
		}

		roleForLogs := resolveRoleForLogs(roleResult.Role, task.Role)
		if resolutionResult.Success {
			result.TasksResolved++
			emitTaskComplete(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve))
		} else {
			result.TasksBlocked++
			if resolutionResult.TimedOut {
				emitTaskTimeout(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve), resolutionResult.BlockReason, cfg.Timeouts.WorkerSeconds)
			} else {
				emitTaskFailure(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve), resolutionResult.BlockReason)
			}
		}
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
	stageInput := worker.StageInput{
		RepoRoot:     repoRoot,
		WorktreeRoot: worktreePath,
		Task:         task,
		Stage:        roles.StageResolve,
		Role:         roleResult.Role, // Use selected role for conflict resolution
		Warn: func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	}

	stageResult, err := worker.StageEnvAndPrompts(stageInput)
	if err != nil {
		return worker.IngestResult{}, roleResult, fmt.Errorf("stage conflict resolution environment: %w", err)
	}

	roleForLogs := resolveRoleForLogs(roleResult.Role, task.Role)
	emitTaskStart(opts.Stdout, task.ID, roleForLogs, string(roles.StageResolve))

	// Execute the conflict resolution worker
	execResult, err := worker.ExecuteWorkerFromConfigWithAudit(cfg, task, stageResult, worktreePath, func(msg string) {
		fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
	}, auditor, worktreePath)
	if err != nil {
		return worker.IngestResult{}, roleResult, fmt.Errorf("execute conflict resolution worker: %w", err)
	}

	// Ingest the worker result
	ingestInput := worker.IngestInput{
		TaskID:       task.ID,
		WorktreePath: worktreePath,
		Stage:        roles.StageResolve,
		ExecResult:   execResult,
		Warn: func(msg string) {
			fmt.Fprintf(opts.Stderr, "Warning: %s\n", msg)
		},
	}

	ingestResult, err := worker.IngestWorkerResult(ingestInput)
	if err != nil {
		return worker.IngestResult{}, roleResult, fmt.Errorf("ingest conflict resolution result: %w", err)
	}

	return ingestResult, roleResult, nil
}

// UpdateTaskStateFromConflictResolution updates the task index based on conflict resolution results.
func UpdateTaskStateFromConflictResolution(idx *index.Index, taskID string, resolutionResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateConflict
	if resolutionResult.Success {
		target = resolutionResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
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

func isRoleInFlight(state index.TaskState) bool {
	switch state {
	case index.TaskStateWorked, index.TaskStateTested, index.TaskStateConflict, index.TaskStateResolved:
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
func ExecuteMergeStage(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (MergeStageResult, error) {
	result := MergeStageResult{}

	selectedTasks, err := selectTasksForStage(*idx, caps, index.TaskStateResolved)
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
		worktreePath, err := manager.WorktreePath(task.ID, task.Attempts.Total)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
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

func selectTasksForStage(idx index.Index, caps scheduler.RoleCaps, states ...index.TaskState) ([]index.Task, error) {
	if len(states) == 0 {
		return nil, nil
	}
	ordered, err := scheduler.OrderedEligibleTasks(idx, nil)
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

// UpdateTaskStateFromMerge updates the task index based on merge results.
func UpdateTaskStateFromMerge(idx *index.Index, taskID string, mergeResult worker.IngestResult, auditor index.TransitionAuditor) error {
	target := index.TaskStateConflict
	if mergeResult.Success {
		target = mergeResult.NewState
	}
	if err := applyTaskStateTransition(idx, taskID, target, auditor); err != nil {
		return fmt.Errorf("task %q: %w", taskID, err)
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
		if task.State == index.TaskStateOpen {
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
		branchName := branchManager.GetTaskBranchName(task.ID)
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
