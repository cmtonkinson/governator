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
	ID            string          `json:"id"`
	Title         string          `json:"title,omitempty"`
	Path          string          `json:"path"`
	Kind          TaskKind        `json:"kind"`
	State         TaskState       `json:"state"`
	Role          Role            `json:"role"`
	AssignedRole  string          `json:"assigned_role,omitempty"`
	BlockedReason string          `json:"blocked,omitempty"`
	MergeConflict bool            `json:"merge_conflict,omitempty"`
	PID           int             `json:"pid,omitempty"`
	Dependencies  []string        `json:"dependencies"`
	Retries       RetryPolicy     `json:"retries"`
	Attempts      AttemptCounters `json:"attempts"`
	Metrics       ExecutionMetrics `json:"metrics,omitempty"`
	Order         int             `json:"order"`
	Overlap       []string        `json:"overlap"`
}

// TaskState labels the lifecycle state for a task.
type TaskState = state.TaskState

const (
	// TaskStateBacklog indicates the task is awaiting triage.
	TaskStateBacklog TaskState = state.TaskStateBacklog
	// TaskStateTriaged indicates the task has been triaged and awaits implementation.
	TaskStateTriaged TaskState = state.TaskStateTriaged
	// TaskStateImplemented indicates the task has implementation work done and awaits testing.
	TaskStateImplemented TaskState = state.TaskStateImplemented
	// TaskStateTested indicates the task has been tested and awaits review.
	TaskStateTested TaskState = state.TaskStateTested
	// TaskStateReviewed indicates the task has been reviewed and is merge-ready.
	TaskStateReviewed TaskState = state.TaskStateReviewed
	// TaskStateMergeable indicates the task is ready to be merged.
	TaskStateMergeable TaskState = state.TaskStateMergeable
	// TaskStateMerged indicates the task has been merged into main.
	TaskStateMerged TaskState = state.TaskStateMerged
	// TaskStateBlocked indicates the task cannot proceed without intervention.
	TaskStateBlocked TaskState = state.TaskStateBlocked
	// TaskStateConflict indicates the task has a merge or execution conflict.
	TaskStateConflict TaskState = state.TaskStateConflict
	// TaskStateResolved indicates a previously conflicted task has been resolved.
	TaskStateResolved TaskState = state.TaskStateResolved
	// Backwards compatibility aliases
	TaskStateOpen   TaskState = TaskStateTriaged
	TaskStateWorked TaskState = TaskStateImplemented
	TaskStateDone   TaskState = TaskStateMerged
)

// Role names the worker role assigned to a task.
type Role string

// TaskKind distinguishes planning work from execution work.
type TaskKind string

const (
	// TaskKindPlanning identifies a planning step in the planning workstream.
	TaskKindPlanning TaskKind = "planning"
	// TaskKindExecution identifies implementation work run during execution.
	TaskKindExecution TaskKind = "execution"
)

// RetryPolicy defines the retry limits for a task.
type RetryPolicy struct {
	MaxAttempts int `json:"max_attempts"`
}

// AttemptCounters tracks how many attempts have been made.
type AttemptCounters struct {
	Total  int `json:"total"`
	Failed int `json:"failed"`
}

// ExecutionMetrics captures execution statistics for retrospective analysis.
type ExecutionMetrics struct {
	DurationMs     int64 `json:"duration_ms,omitempty"`      // Total wall time in milliseconds across all stages
	TokensPrompt   int   `json:"tokens_prompt,omitempty"`    // Total input tokens consumed
	TokensResponse int   `json:"tokens_response,omitempty"`  // Total output tokens generated
	TokensTotal    int   `json:"tokens_total,omitempty"`     // Total tokens (prompt + response)
}
