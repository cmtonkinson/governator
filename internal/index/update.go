// Package index provides helpers for updating task state and attempts.
package index

import (
	"fmt"
	"log"

	"github.com/cmtonkinson/governator/internal/state"
)

// TransitionAuditor records task lifecycle transitions for audit logging.
type TransitionAuditor interface {
	LogTaskTransition(taskID string, role string, from string, to string) error
}

// TransitionTaskToImplemented moves a task from triaged to implemented.
func TransitionTaskToImplemented(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateImplemented)
}

// TransitionTaskToTested moves a task from implemented to tested.
func TransitionTaskToTested(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateTested)
}

// TransitionTaskToMerged moves a task from mergeable or resolved to merged.
func TransitionTaskToMerged(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateMerged)
}

// TransitionTaskToBlocked moves a task from triaged, implemented, tested, mergeable, conflict, or resolved to blocked.
func TransitionTaskToBlocked(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateBlocked)
}

// TransitionTaskToConflict moves a task from tested, mergeable, or resolved to conflict.
func TransitionTaskToConflict(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateConflict)
}

// TransitionTaskToResolved moves a task from conflict to resolved.
func TransitionTaskToResolved(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateResolved)
}

// TransitionTaskToTriaged moves a task from blocked to triaged.
func TransitionTaskToTriaged(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateTriaged)
}

// TransitionTaskToReviewed marks a tested task as reviewed.
func TransitionTaskToReviewed(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateReviewed)
}

// TransitionTaskToMergeable marks a reviewed or resolved task as mergeable.
func TransitionTaskToMergeable(idx *Index, taskID string) error {
	return transitionTaskState(idx, taskID, TaskStateMergeable)
}

// Legacy wrappers kept for compatibility.
func TransitionTaskToWorked(idx *Index, taskID string) error {
	return TransitionTaskToImplemented(idx, taskID)
}

func TransitionTaskToDone(idx *Index, taskID string) error {
	return TransitionTaskToMerged(idx, taskID)
}

func TransitionTaskToOpen(idx *Index, taskID string) error {
	return TransitionTaskToTriaged(idx, taskID)
}

// TransitionTaskStateWithAudit moves a task to the target state and records an audit entry.
func TransitionTaskStateWithAudit(idx *Index, taskID string, to TaskState, auditor TransitionAuditor) error {
	task, err := findTaskByID(idx, taskID)
	if err != nil {
		return err
	}
	from := task.State
	if err := state.ValidateTransition(task.State, to); err != nil {
		wrapped := fmt.Errorf("task %q: %w", taskID, err)
		log.Printf("task %q transition from %q to %q rejected: %v", taskID, task.State, to, wrapped)
		return wrapped
	}
	task.State = to
	if auditor != nil {
		if err := auditor.LogTaskTransition(task.ID, string(task.Role), string(from), string(to)); err != nil {
			log.Printf("task %q transition audit log failed: %v", taskID, err)
		}
	}
	return nil
}

// IncrementTaskAttempt increments the total attempt counter for a task.
func IncrementTaskAttempt(idx *Index, taskID string) error {
	task, err := findTaskByID(idx, taskID)
	if err != nil {
		return err
	}
	task.Attempts.Total++
	return nil
}

// IncrementTaskFailedAttempt increments the failed attempt counter for a task.
func IncrementTaskFailedAttempt(idx *Index, taskID string) error {
	task, err := findTaskByID(idx, taskID)
	if err != nil {
		return err
	}
	task.Attempts.Failed++
	return nil
}

// transitionTaskState enforces lifecycle state transitions before updating a task.
func transitionTaskState(idx *Index, taskID string, to TaskState) error {
	return TransitionTaskStateWithAudit(idx, taskID, to, nil)
}

// findTaskByID locates a task in the index and validates the inputs.
func findTaskByID(idx *Index, taskID string) (*Task, error) {
	if idx == nil {
		return nil, fmt.Errorf("index is nil")
	}
	if taskID == "" {
		return nil, fmt.Errorf("task id is required")
	}
	for i := range idx.Tasks {
		if idx.Tasks[i].ID == taskID {
			return &idx.Tasks[i], nil
		}
	}
	return nil, fmt.Errorf("task %q not found in index", taskID)
}
