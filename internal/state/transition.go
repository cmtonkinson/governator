// Package state defines lifecycle state machine types and transition guards.
package state

import "fmt"

// TaskState labels the lifecycle state for a task.
type TaskState string

const (
	// TaskStateBacklog indicates the task is still awaiting triage.
	TaskStateBacklog TaskState = "backlog"
	// TaskStateTriaged indicates the task has been triaged and is ready for implementation.
	TaskStateTriaged TaskState = "triaged"
	// TaskStateImplemented indicates the task has implementation work done and awaits testing.
	TaskStateImplemented TaskState = "implemented"
	// TaskStateTested indicates the task has been tested and awaits review.
	TaskStateTested TaskState = "tested"
	// TaskStateReviewed indicates the task has been reviewed and is mergeable.
	TaskStateReviewed TaskState = "reviewed"
	// TaskStateMergeable indicates the task is ready to be merged.
	TaskStateMergeable TaskState = "mergeable"
	// TaskStateMerged indicates the task has been merged into main.
	TaskStateMerged TaskState = "merged"
	// TaskStateBlocked indicates the task cannot proceed without intervention.
	TaskStateBlocked TaskState = "blocked"
	// TaskStateConflict indicates the task has a merge or execution conflict.
	TaskStateConflict TaskState = "conflict"
	// TaskStateResolved indicates a previously conflicted task has been resolved.
	TaskStateResolved TaskState = "resolved"
)

// allowedTransitions defines the permitted lifecycle state changes.
var allowedTransitions = map[TaskState]map[TaskState]struct{}{
	TaskStateBacklog: {
		TaskStateTriaged: {},
	},
	TaskStateTriaged: {
		TaskStateImplemented: {},
		TaskStateBlocked:     {},
	},
	TaskStateImplemented: {
		TaskStateTested:  {},
		TaskStateBlocked: {},
	},
	TaskStateTested: {
		TaskStateReviewed: {},
		TaskStateConflict: {},
		TaskStateTriaged:  {},
		TaskStateBlocked:  {},
	},
	TaskStateReviewed: {
		TaskStateMergeable: {},
		TaskStateBlocked:   {},
	},
	TaskStateMergeable: {
		TaskStateMerged:   {},
		TaskStateConflict: {},
		TaskStateBlocked:  {},
	},
	TaskStateMerged: {},
	TaskStateConflict: {
		TaskStateResolved: {},
		TaskStateBlocked:  {},
	},
	TaskStateResolved: {
		TaskStateMergeable: {},
		TaskStateConflict:  {},
	},
	TaskStateBlocked: {
		TaskStateTriaged: {},
	},
}

const (
	// Deprecated aliases for legacy state names.
	TaskStateOpen   = TaskStateTriaged
	TaskStateWorked = TaskStateImplemented
	TaskStateDone   = TaskStateMerged
)

// IsValidTransition reports whether the lifecycle allows the requested change.
func IsValidTransition(from TaskState, to TaskState) bool {
	if from == "" || to == "" {
		return false
	}
	allowed, ok := allowedTransitions[from]
	if !ok {
		return false
	}
	_, ok = allowed[to]
	return ok
}

// ValidateTransition returns an error when a lifecycle change is not allowed.
func ValidateTransition(from TaskState, to TaskState) error {
	if !IsValidTransition(from, to) {
		return fmt.Errorf("invalid task state transition from %q to %q", from, to)
	}
	return nil
}
