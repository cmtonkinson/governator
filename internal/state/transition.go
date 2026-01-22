// Package state defines lifecycle state machine types and transition guards.
package state

import "fmt"

// TaskState labels the lifecycle state for a task.
type TaskState string

const (
	// TaskStateOpen indicates the task has not been started.
	TaskStateOpen TaskState = "open"
	// TaskStateWorked indicates the task has work completed and awaits testing.
	TaskStateWorked TaskState = "worked"
	// TaskStateTested indicates the task has been tested and awaits review.
	TaskStateTested TaskState = "tested"
	// TaskStateDone indicates the task is complete.
	TaskStateDone TaskState = "done"
	// TaskStateBlocked indicates the task cannot proceed without intervention.
	TaskStateBlocked TaskState = "blocked"
	// TaskStateConflict indicates the task has a merge or execution conflict.
	TaskStateConflict TaskState = "conflict"
	// TaskStateResolved indicates a previously conflicted task has been resolved.
	TaskStateResolved TaskState = "resolved"
)

// allowedTransitions defines the permitted lifecycle state changes.
var allowedTransitions = map[TaskState]map[TaskState]struct{}{
	TaskStateOpen: {
		TaskStateWorked:  {},
		TaskStateBlocked: {},
	},
	TaskStateWorked: {
		TaskStateTested:  {},
		TaskStateBlocked: {},
	},
	TaskStateTested: {
		TaskStateDone:     {},
		TaskStateConflict: {},
		TaskStateOpen:     {},
		TaskStateBlocked:  {},
	},
	TaskStateConflict: {
		TaskStateResolved: {},
		TaskStateBlocked:  {},
	},
	TaskStateResolved: {
		TaskStateDone:     {},
		TaskStateConflict: {},
	},
	TaskStateBlocked: {
		TaskStateOpen: {},
	},
}

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
