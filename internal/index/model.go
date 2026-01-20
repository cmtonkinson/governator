// Package index defines the task index data model and JSON mapping.
package index

import "github.com/cmtonkinson/governator/internal/state"

// Index represents the canonical task index persisted as JSON.
type Index struct {
	SchemaVersion int     `json:"schema_version"`
	Digests       Digests `json:"digests"`
	Tasks         []Task  `json:"tasks"`
}

// Digests captures content digests for planning artifacts and governance docs.
type Digests struct {
	GovernatorMD string            `json:"governator_md"`
	PlanningDocs map[string]string `json:"planning_docs"`
}

// Task captures a single task entry from the task index.
type Task struct {
	ID           string          `json:"id"`
	Title        string          `json:"title,omitempty"`
	Path         string          `json:"path"`
	State        TaskState       `json:"state"`
	Role         Role            `json:"role"`
	Dependencies []string        `json:"dependencies"`
	Retries      RetryPolicy     `json:"retries"`
	Attempts     AttemptCounters `json:"attempts"`
	Order        int             `json:"order"`
	Overlap      []string        `json:"overlap"`
}

// TaskState labels the lifecycle state for a task.
type TaskState = state.TaskState

const (
	// TaskStateOpen indicates the task has not been started.
	TaskStateOpen TaskState = state.TaskStateOpen
	// TaskStateWorked indicates the task has work completed and awaits testing.
	TaskStateWorked TaskState = state.TaskStateWorked
	// TaskStateTested indicates the task has been tested and awaits review.
	TaskStateTested TaskState = state.TaskStateTested
	// TaskStateDone indicates the task is complete.
	TaskStateDone TaskState = state.TaskStateDone
	// TaskStateBlocked indicates the task cannot proceed without intervention.
	TaskStateBlocked TaskState = state.TaskStateBlocked
	// TaskStateConflict indicates the task has a merge or execution conflict.
	TaskStateConflict TaskState = state.TaskStateConflict
	// TaskStateResolved indicates a previously conflicted task has been resolved.
	TaskStateResolved TaskState = state.TaskStateResolved
)

// Role names the worker role assigned to a task.
type Role string

// RetryPolicy defines the retry limits for a task.
type RetryPolicy struct {
	MaxAttempts int `json:"max_attempts"`
}

// AttemptCounters tracks how many attempts have been made.
type AttemptCounters struct {
	Total  int `json:"total"`
	Failed int `json:"failed"`
}
