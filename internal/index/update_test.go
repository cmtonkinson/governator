// Tests for task index update helpers.
package index

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

// TestTransitionHappyPath moves a task through triaged, implemented, tested, and merged.
func TestTransitionHappyPath(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateTriaged,
				Role:  "builder",
			},
		},
	}

	if err := TransitionTaskToImplemented(&idx, "task-1"); err != nil {
		t.Fatalf("transition to implemented: %v", err)
	}
	if err := TransitionTaskToTested(&idx, "task-1"); err != nil {
		t.Fatalf("transition to tested: %v", err)
	}
	if err := TransitionTaskToReviewed(&idx, "task-1"); err != nil {
		t.Fatalf("transition to reviewed: %v", err)
	}
	if err := TransitionTaskToMergeable(&idx, "task-1"); err != nil {
		t.Fatalf("transition to mergeable: %v", err)
	}
	if err := TransitionTaskToMerged(&idx, "task-1"); err != nil {
		t.Fatalf("transition to merged: %v", err)
	}

	if got := idx.Tasks[0].State; got != TaskStateMerged {
		t.Fatalf("expected final state %q, got %q", TaskStateMerged, got)
	}
	if got := idx.Tasks[0].ID; got != "task-1" {
		t.Fatalf("expected task id %q, got %q", "task-1", got)
	}
	if got := len(idx.Tasks); got != 1 {
		t.Fatalf("expected 1 task in index, got %d", got)
	}
}

// TestTransitionConflictResolutionFlow moves a task through conflict resolution to done.
func TestTransitionConflictResolutionFlow(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateTriaged,
				Role:  "builder",
			},
		},
	}

	if err := TransitionTaskToImplemented(&idx, "task-1"); err != nil {
		t.Fatalf("transition to implemented: %v", err)
	}
	if err := TransitionTaskToTested(&idx, "task-1"); err != nil {
		t.Fatalf("transition to tested: %v", err)
	}
	if err := TransitionTaskToConflict(&idx, "task-1"); err != nil {
		t.Fatalf("transition to conflict: %v", err)
	}
	if err := TransitionTaskToResolved(&idx, "task-1"); err != nil {
		t.Fatalf("transition to resolved: %v", err)
	}
	if err := TransitionTaskToMergeable(&idx, "task-1"); err != nil {
		t.Fatalf("transition to mergeable: %v", err)
	}
	if err := TransitionTaskToMerged(&idx, "task-1"); err != nil {
		t.Fatalf("transition to merged: %v", err)
	}

	if got := idx.Tasks[0].State; got != TaskStateDone {
		t.Fatalf("expected final state %q, got %q", TaskStateDone, got)
	}
}

// TestTransitionBlockedReset ensures a blocked task can be reset to open.
func TestTransitionBlockedReset(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateBlocked,
				Role:  "builder",
			},
		},
	}

	if err := TransitionTaskToTriaged(&idx, "task-1"); err != nil {
		t.Fatalf("transition to triaged: %v", err)
	}
	if got := idx.Tasks[0].State; got != TaskStateTriaged {
		t.Fatalf("expected state %q, got %q", TaskStateTriaged, got)
	}
}

// TestIncrementTaskAttempt ensures the attempt counter increments.
func TestIncrementTaskAttempt(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateTriaged,
				Role:  "builder",
				Attempts: AttemptCounters{
					Total: 1,
				},
			},
		},
	}

	if err := IncrementTaskAttempt(&idx, "task-1"); err != nil {
		t.Fatalf("increment attempt: %v", err)
	}
	if got := idx.Tasks[0].Attempts.Total; got != 2 {
		t.Fatalf("expected total attempts 2, got %d", got)
	}
}

// TestIncrementTaskFailedAttempt ensures the failed attempt counter increments.
func TestIncrementTaskFailedAttempt(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateTriaged,
				Role:  "builder",
				Attempts: AttemptCounters{
					Failed: 1,
				},
			},
		},
	}

	if err := IncrementTaskFailedAttempt(&idx, "task-1"); err != nil {
		t.Fatalf("increment failed attempt: %v", err)
	}
	if got := idx.Tasks[0].Attempts.Failed; got != 2 {
		t.Fatalf("expected failed attempts 2, got %d", got)
	}
}

// TestTransitionFromDoneToWorkedFails rejects invalid transitions.
func TestTransitionFromDoneToWorkedFails(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateMerged,
				Role:  "builder",
			},
		},
	}

	var buf bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	err := TransitionTaskToImplemented(&idx, "task-1")
	if err == nil {
		t.Fatal("expected error for done to worked transition")
	}
	if !strings.Contains(err.Error(), "invalid task state transition") {
		t.Fatalf("expected transition error, got %v", err)
	}
	if !strings.Contains(buf.String(), "transition from") {
		t.Fatalf("expected transition log entry, got %q", buf.String())
	}
}

// TestTransitionTaskStateWithAuditLogs ensures audit logging is invoked on transitions.
func TestTransitionTaskStateWithAuditLogs(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateTriaged,
				Role:  "builder",
			},
		},
	}

	auditor := &transitionAuditCollector{}
	if err := TransitionTaskStateWithAudit(&idx, "task-1", TaskStateImplemented, auditor); err != nil {
		t.Fatalf("transition with audit: %v", err)
	}
	if got := idx.Tasks[0].State; got != TaskStateImplemented {
		t.Fatalf("expected state %q, got %q", TaskStateImplemented, got)
	}
	if len(auditor.calls) != 1 {
		t.Fatalf("expected 1 audit call, got %d", len(auditor.calls))
	}
	call := auditor.calls[0]
	if call.from != "triaged" || call.to != "implemented" {
		t.Fatalf("unexpected transition audit call: %#v", call)
	}
}

// TestTransitionTaskStateWithAuditIgnoresAuditFailures ensures audit errors do not fail transitions.
func TestTransitionTaskStateWithAuditIgnoresAuditFailures(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateTriaged,
				Role:  "builder",
			},
		},
	}

	var buf bytes.Buffer
	prevOutput := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOutput)
		log.SetFlags(prevFlags)
	}()

	auditor := &transitionAuditCollector{err: errors.New("audit down")}
	if err := TransitionTaskStateWithAudit(&idx, "task-1", TaskStateImplemented, auditor); err != nil {
		t.Fatalf("transition with audit failure: %v", err)
	}
	if !strings.Contains(buf.String(), "transition audit log failed") {
		t.Fatalf("expected audit log failure warning, got %q", buf.String())
	}
}

// transitionAuditCollector records transition audit calls for testing.
type transitionAuditCollector struct {
	calls []transitionAuditCall
	err   error
}

// transitionAuditCall captures one audit invocation.
type transitionAuditCall struct {
	taskID string
	role   string
	from   string
	to     string
}

// LogTaskTransition records audit calls for testing.
func (collector *transitionAuditCollector) LogTaskTransition(taskID string, role string, from string, to string) error {
	if collector == nil {
		return errors.New("collector is nil")
	}
	collector.calls = append(collector.calls, transitionAuditCall{
		taskID: taskID,
		role:   role,
		from:   from,
		to:     to,
	})
	return collector.err
}
