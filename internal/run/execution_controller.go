// Package run defines the execution workstream controller implementation.
package run

import (
	"fmt"
	"strings"

	"github.com/cmtonkinson/governator/internal/audit"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/scheduler"
)

// executionStage labels the ordered execution stages.
type executionStage string

const (
	// executionStageWork runs implementation workers.
	executionStageWork executionStage = "work"
	// executionStageTest runs testing workers.
	executionStageTest executionStage = "test"
	// executionStageReview runs review workers.
	executionStageReview executionStage = "review"
	// executionStageResolve runs conflict resolution workers.
	executionStageResolve executionStage = "resolve"
	// executionStageMerge merges resolved work.
	executionStageMerge executionStage = "merge"
	// executionStageBranch ensures branches exist for open tasks.
	executionStageBranch executionStage = "branch"
)

// executionController adapts task execution stages to the workstream runner.
type executionController struct {
	repoRoot           string
	idx                *index.Index
	cfg                config.Config
	caps               scheduler.RoleCaps
	inFlight           inflight.Set
	resumeWorktrees    map[string]string
	worktreeOverrides  map[string]string
	transitionAuditor  index.TransitionAuditor
	workerAuditor      *audit.Logger
	opts               Options
	baseBranch         string
	cursor             int
	stages             []executionStage
	workResult         WorkStageResult
	testResult         TestStageResult
	reviewResult       ReviewStageResult
	conflictResult     ConflictResolutionStageResult
	mergeResult        MergeStageResult
	branchResult       BranchStageResult
	inFlightWasUpdated bool
}

// newExecutionController constructs a controller for the execution workstream.
func newExecutionController(repoRoot string, idx *index.Index, cfg config.Config, caps scheduler.RoleCaps, inFlight inflight.Set, resumeWorktrees map[string]string, transitionAuditor index.TransitionAuditor, workerAuditor *audit.Logger, opts Options, baseBranch string) *executionController {
	seededOverrides := mergeWorktreeOverrides(resumeWorktrees, nil)
	return &executionController{
		repoRoot:          repoRoot,
		idx:               idx,
		cfg:               cfg,
		caps:              caps,
		inFlight:          inFlight,
		resumeWorktrees:   seededOverrides,
		worktreeOverrides: seededOverrides,
		transitionAuditor: transitionAuditor,
		workerAuditor:     workerAuditor,
		opts:              opts,
		baseBranch:        baseBranch,
		stages: []executionStage{
			executionStageWork,
			executionStageTest,
			executionStageReview,
			executionStageResolve,
			executionStageMerge,
			executionStageBranch,
		},
	}
}

// CurrentStep returns the active execution stage as a workstream step.
func (controller *executionController) CurrentStep() (workstreamStep, bool, error) {
	if controller.cursor >= len(controller.stages) {
		return workstreamStep{}, false, nil
	}
	stage := controller.stages[controller.cursor]
	return workstreamStep{name: string(stage)}, true, nil
}

// Collect is a no-op for execution stages, which collect during dispatch.
func (controller *executionController) Collect(step workstreamStep) (workstreamCollectResult, error) {
	return workstreamCollectResult{}, nil
}

// Advance is unused for execution stages, which move during dispatch.
func (controller *executionController) Advance(step workstreamStep, collect workstreamCollectResult) (bool, error) {
	return false, nil
}

// GateBeforeDispatch is a no-op for execution stages.
func (controller *executionController) GateBeforeDispatch(step workstreamStep) error {
	return nil
}

// Dispatch runs the stage associated with the current step.
func (controller *executionController) Dispatch(step workstreamStep) (workstreamDispatchResult, error) {
	stage, err := controller.stageForStep(step)
	if err != nil {
		return workstreamDispatchResult{}, err
	}

	result := workstreamDispatchResult{Continue: true}
	switch stage {
	case executionStageWork:
		workResult, err := ExecuteWorkStage(controller.repoRoot, controller.idx, controller.cfg, controller.caps, controller.inFlight, controller.resumeWorktrees, controller.transitionAuditor, controller.workerAuditor, controller.opts)
		if err != nil {
			return workstreamDispatchResult{}, err
		}
		controller.workResult = workResult
		controller.worktreeOverrides = mergeWorktreeOverrides(controller.resumeWorktrees, workResult.WorktreePaths)
		controller.markInFlightUpdated(workResult.InFlightUpdated)
		result.Handled = workResult.TasksDispatched > 0 || workResult.TasksWorked > 0 || workResult.TasksBlocked > 0
	case executionStageTest:
		testResult, err := ExecuteTestStage(controller.repoRoot, controller.idx, controller.cfg, controller.caps, controller.inFlight, controller.worktreeOverrides, controller.transitionAuditor, controller.workerAuditor, controller.opts)
		if err != nil {
			return workstreamDispatchResult{}, err
		}
		controller.testResult = testResult
		controller.markInFlightUpdated(testResult.InFlightUpdated)
		result.Handled = testResult.TasksDispatched > 0 || testResult.TasksTested > 0 || testResult.TasksBlocked > 0
	case executionStageReview:
		reviewResult, err := ExecuteReviewStage(controller.repoRoot, controller.idx, controller.cfg, controller.caps, controller.inFlight, controller.worktreeOverrides, controller.transitionAuditor, controller.workerAuditor, controller.opts)
		if err != nil {
			return workstreamDispatchResult{}, err
		}
		controller.reviewResult = reviewResult
		controller.markInFlightUpdated(reviewResult.InFlightUpdated)
		result.Handled = reviewResult.TasksDispatched > 0 || reviewResult.TasksReviewed > 0 || reviewResult.TasksBlocked > 0
	case executionStageResolve:
		conflictResult, err := ExecuteConflictResolutionStage(controller.repoRoot, controller.idx, controller.cfg, controller.caps, controller.inFlight, controller.worktreeOverrides, controller.transitionAuditor, controller.workerAuditor, controller.opts)
		if err != nil {
			return workstreamDispatchResult{}, err
		}
		controller.conflictResult = conflictResult
		controller.markInFlightUpdated(conflictResult.InFlightUpdated)
		result.Handled = conflictResult.TasksDispatched > 0 || conflictResult.TasksResolved > 0 || conflictResult.TasksBlocked > 0
	case executionStageMerge:
		mergeResult, err := ExecuteMergeStage(controller.repoRoot, controller.idx, controller.cfg, controller.caps, controller.worktreeOverrides, controller.transitionAuditor, controller.workerAuditor, controller.opts)
		if err != nil {
			return workstreamDispatchResult{}, err
		}
		controller.mergeResult = mergeResult
		result.Handled = mergeResult.TasksProcessed > 0
	case executionStageBranch:
		branchResult, err := EnsureBranchesForOpenTasks(controller.repoRoot, controller.idx, controller.workerAuditor, controller.opts, controller.baseBranch)
		if err != nil {
			return workstreamDispatchResult{}, err
		}
		controller.branchResult = branchResult
		result.Handled = branchResult.BranchesCreated > 0
	default:
		return workstreamDispatchResult{}, fmt.Errorf("unsupported execution stage %q", stage)
	}

	controller.cursor++
	result.Continue = controller.cursor < len(controller.stages)
	return result, nil
}

// EmitRunning is a no-op for execution stages.
func (controller *executionController) EmitRunning(step workstreamStep, pids []int) {}

// EmitAgentComplete is a no-op for execution stages.
func (controller *executionController) EmitAgentComplete(step workstreamStep, collect workstreamCollectResult) {}

// stageForStep resolves the execution stage encoded in the step name.
func (controller *executionController) stageForStep(step workstreamStep) (executionStage, error) {
	name := strings.TrimSpace(step.name)
	if name == "" {
		return "", fmt.Errorf("execution step name is required")
	}
	stage := executionStage(name)
	switch stage {
	case executionStageWork, executionStageTest, executionStageReview, executionStageResolve, executionStageMerge, executionStageBranch:
		return stage, nil
	default:
		return "", fmt.Errorf("unknown execution stage %q", name)
	}
}

// markInFlightUpdated tracks if any stage updated the in-flight set.
func (controller *executionController) markInFlightUpdated(updated bool) {
	if updated {
		controller.inFlightWasUpdated = true
	}
}
