// Package run provides the main orchestration logic for the run command.
package run

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/state"
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

	// Load task index
	indexPath := filepath.Join(repoRoot, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		return Result{}, fmt.Errorf("load task index: %w", err)
	}

	// Check for planning drift
	if err := CheckPlanningDrift(repoRoot, idx.Digests); err != nil {
		return Result{}, err
	}

	// Set up audit logging
	auditor, err := audit.NewLogger(repoRoot, opts.Stderr)
	if err != nil {
		return Result{}, fmt.Errorf("create audit logger: %w", err)
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
	testResult, err := ExecuteTestStage(repoRoot, &idx, cfg, auditor, auditor, opts)
	if err != nil {
		return Result{}, fmt.Errorf("execute test stage: %w", err)
	}

	// Execute review stage for tested tasks
	reviewResult, err := ExecuteReviewStage(repoRoot, &idx, cfg, auditor, auditor, opts)
	if err != nil {
		return Result{}, fmt.Errorf("execute review stage: %w", err)
	}

	// Execute conflict resolution stage for resolved tasks
	conflictResult, err := ExecuteConflictResolutionStage(repoRoot, &idx, cfg, auditor, auditor, opts)
	if err != nil {
		return Result{}, fmt.Errorf("execute conflict resolution stage: %w", err)
	}

	// Save updated index
	if len(resumedTasks) > 0 || len(blockedTasks) > 0 || testResult.TasksProcessed > 0 || reviewResult.TasksProcessed > 0 || conflictResult.TasksProcessed > 0 {
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
func ExecuteTestStage(repoRoot string, idx *index.Index, cfg config.Config, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (TestStageResult, error) {
	result := TestStageResult{}

	// Find tasks eligible for testing (in worked state)
	var workedTasks []index.Task
	for _, task := range idx.Tasks {
		if task.State == index.TaskStateWorked {
			workedTasks = append(workedTasks, task)
		}
	}

	if len(workedTasks) == 0 {
		return result, nil
	}

	// Set up worktree manager
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	// Process each worked task through test stage
	for _, task := range workedTasks {
		result.TasksProcessed++

		// Get the worktree path for the task
		worktreePath, err := manager.WorktreePath(task.ID, task.Attempts.Total)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

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
				fmt.Fprintf(opts.Stdout, "Task %s blocked: %s\n", task.ID, failedResult.BlockReason)
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
			fmt.Fprintf(opts.Stdout, "Task %s tested successfully\n", task.ID)
		} else {
			result.TasksBlocked++
			fmt.Fprintf(opts.Stdout, "Task %s blocked: %s\n", task.ID, testResult.BlockReason)
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
	// Find the task in the index
	taskIndex := -1
	for i, task := range idx.Tasks {
		if task.ID == taskID {
			taskIndex = i
			break
		}
	}

	if taskIndex == -1 {
		return fmt.Errorf("task %s not found in index", taskID)
	}

	task := &idx.Tasks[taskIndex]
	oldState := task.State

	// Update task state based on test result
	if testResult.Success {
		task.State = testResult.NewState // Should be TaskStateTested
	} else {
		task.State = index.TaskStateBlocked
		// Note: BlockReason is not persisted in the task index, only used for logging
	}

	// Validate the state transition
	if err := state.ValidateTransition(oldState, task.State); err != nil {
		return fmt.Errorf("invalid state transition for task %s: %w", taskID, err)
	}

	// Log the state change to audit
	if auditor != nil {
		_ = auditor.LogTaskTransition(taskID, string(task.Role), string(oldState), string(task.State))
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
func ExecuteReviewStage(repoRoot string, idx *index.Index, cfg config.Config, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (ReviewStageResult, error) {
	result := ReviewStageResult{}

	// Find tasks eligible for review (in tested state)
	var testedTasks []index.Task
	for _, task := range idx.Tasks {
		if task.State == index.TaskStateTested {
			testedTasks = append(testedTasks, task)
		}
	}

	if len(testedTasks) == 0 {
		return result, nil
	}

	// Set up worktree manager
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	// Process each tested task through review stage
	for _, task := range testedTasks {
		result.TasksProcessed++

		// Get the worktree path for the task
		worktreePath, err := manager.WorktreePath(task.ID, task.Attempts.Total)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

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
				fmt.Fprintf(opts.Stdout, "Task %s blocked: %s\n", task.ID, failedResult.BlockReason)
			}
			continue
		}

		// If review was successful, execute the merge flow
		if reviewResult.Success {
			mergeInput := MergeFlowInput{
				RepoRoot:     repoRoot,
				WorktreePath: worktreePath,
				Task:         task,
				MainBranch:   "main", // TODO: Make this configurable
				Auditor:      workerAuditor,
			}

			mergeResult, err := ExecuteReviewMergeFlow(mergeInput)
			if err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to execute merge flow for task %s: %v\n", task.ID, err)
				// Create a blocked result for merge flow failure
				failedResult := worker.IngestResult{
					Success:     false,
					NewState:    index.TaskStateBlocked,
					BlockReason: fmt.Sprintf("merge flow failed: %v", err),
				}
				if updateErr := UpdateTaskStateFromReviewResult(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
					fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
				} else {
					result.TasksBlocked++
					fmt.Fprintf(opts.Stdout, "Task %s blocked: %s\n", task.ID, failedResult.BlockReason)
				}
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
				result.TasksReviewed++
				fmt.Fprintf(opts.Stdout, "Task %s reviewed and merged successfully\n", task.ID)
			} else {
				// Task moved to conflict state
				fmt.Fprintf(opts.Stdout, "Task %s has merge conflicts: %s\n", task.ID, mergeResult.ConflictError)
			}
		} else {
			// Review failed, update task state
			if err := UpdateTaskStateFromReviewResult(idx, task.ID, reviewResult, transitionAuditor); err != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, err)
				continue
			}

			result.TasksBlocked++
			fmt.Fprintf(opts.Stdout, "Task %s blocked: %s\n", task.ID, reviewResult.BlockReason)
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
	// Find the task in the index
	taskIndex := -1
	for i, task := range idx.Tasks {
		if task.ID == taskID {
			taskIndex = i
			break
		}
	}

	if taskIndex == -1 {
		return fmt.Errorf("task %s not found in index", taskID)
	}

	task := &idx.Tasks[taskIndex]
	oldState := task.State

	// Update task state based on review result
	if reviewResult.Success {
		task.State = reviewResult.NewState // Can be TaskStateDone or TaskStateConflict based on merge flow
	} else {
		task.State = index.TaskStateBlocked
		// Note: BlockReason is not persisted in the task index, only used for logging
	}

	// Validate the state transition
	if err := state.ValidateTransition(oldState, task.State); err != nil {
		return fmt.Errorf("invalid state transition for task %s: %w", taskID, err)
	}

	// Log the state change to audit
	if auditor != nil {
		_ = auditor.LogTaskTransition(taskID, string(task.Role), string(oldState), string(task.State))
	}

	return nil
}
// ConflictResolutionStageResult captures the outcome of conflict resolution stage execution.
type ConflictResolutionStageResult struct {
	TasksProcessed int
	TasksResolved  int
	TasksBlocked   int
}

// ExecuteConflictResolutionStage processes tasks in the resolved state through the merge flow.
func ExecuteConflictResolutionStage(repoRoot string, idx *index.Index, cfg config.Config, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options) (ConflictResolutionStageResult, error) {
	result := ConflictResolutionStageResult{}

	// Find tasks eligible for conflict resolution (in resolved state)
	var resolvedTasks []index.Task
	for _, task := range idx.Tasks {
		if task.State == index.TaskStateResolved {
			resolvedTasks = append(resolvedTasks, task)
		}
	}

	if len(resolvedTasks) == 0 {
		return result, nil
	}

	// Set up worktree manager
	manager, err := worktree.NewManager(repoRoot)
	if err != nil {
		return result, fmt.Errorf("create worktree manager: %w", err)
	}

	// Process each resolved task through conflict resolution merge flow
	for _, task := range resolvedTasks {
		result.TasksProcessed++

		// Get the worktree path for the task
		worktreePath, err := manager.WorktreePath(task.ID, task.Attempts.Total)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to get worktree path for task %s: %v\n", task.ID, err)
			continue
		}

		// Execute conflict resolution merge flow
		mergeInput := MergeFlowInput{
			RepoRoot:     repoRoot,
			WorktreePath: worktreePath,
			Task:         task,
			MainBranch:   "main", // TODO: Make this configurable
			Auditor:      workerAuditor,
		}

		mergeResult, err := ExecuteConflictResolutionMergeFlow(mergeInput)
		if err != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to execute conflict resolution merge flow for task %s: %v\n", task.ID, err)
			// Create a blocked result for merge flow failure
			failedResult := worker.IngestResult{
				Success:     false,
				NewState:    index.TaskStateBlocked,
				BlockReason: fmt.Sprintf("conflict resolution merge flow failed: %v", err),
			}
			if updateErr := UpdateTaskStateFromConflictResolution(idx, task.ID, failedResult, transitionAuditor); updateErr != nil {
				fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			} else {
				result.TasksBlocked++
				fmt.Fprintf(opts.Stdout, "Task %s blocked: %s\n", task.ID, failedResult.BlockReason)
			}
			continue
		}

		// Update task state based on merge result
		finalResult := worker.IngestResult{
			Success:     mergeResult.Success,
			NewState:    mergeResult.NewState,
			BlockReason: mergeResult.ConflictError,
		}

		if updateErr := UpdateTaskStateFromConflictResolution(idx, task.ID, finalResult, transitionAuditor); updateErr != nil {
			fmt.Fprintf(opts.Stderr, "Warning: failed to update task state for %s: %v\n", task.ID, updateErr)
			continue
		}

		if mergeResult.Success {
			result.TasksResolved++
			fmt.Fprintf(opts.Stdout, "Task %s conflict resolved and merged successfully\n", task.ID)
		} else {
			// Task moved back to conflict state
			fmt.Fprintf(opts.Stdout, "Task %s still has merge conflicts: %s\n", task.ID, mergeResult.ConflictError)
		}
	}

	return result, nil
}

// UpdateTaskStateFromConflictResolution updates the task index based on conflict resolution results.
func UpdateTaskStateFromConflictResolution(idx *index.Index, taskID string, resolutionResult worker.IngestResult, auditor index.TransitionAuditor) error {
	// Find the task in the index
	taskIndex := -1
	for i, task := range idx.Tasks {
		if task.ID == taskID {
			taskIndex = i
			break
		}
	}

	if taskIndex == -1 {
		return fmt.Errorf("task %s not found in index", taskID)
	}

	task := &idx.Tasks[taskIndex]
	oldState := task.State

	// Update task state based on resolution result
	if resolutionResult.Success {
		task.State = resolutionResult.NewState // Should be TaskStateDone or TaskStateConflict
	} else {
		// For conflict resolution failures, we should transition back to conflict state
		// since resolved can only go to done or conflict, not blocked
		task.State = index.TaskStateConflict
		// Note: BlockReason is not persisted in the task index, only used for logging
	}

	// Validate the state transition
	if err := state.ValidateTransition(oldState, task.State); err != nil {
		return fmt.Errorf("invalid state transition for task %s: %w", taskID, err)
	}

	// Log the state change to audit
	if auditor != nil {
		_ = auditor.LogTaskTransition(taskID, string(task.Role), string(oldState), string(task.State))
	}

	return nil
}