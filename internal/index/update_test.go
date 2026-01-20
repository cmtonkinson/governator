// Tests for task index update helpers.
package index

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

// TestTransitionHappyPath moves a task through open, worked, tested, and done.
func TestTransitionHappyPath(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateOpen,
				Role:  "builder",
			},
		},
	}

	if err := TransitionTaskToWorked(&idx, "task-1"); err != nil {
		t.Fatalf("transition to worked: %v", err)
	}
	if err := TransitionTaskToTested(&idx, "task-1"); err != nil {
		t.Fatalf("transition to tested: %v", err)
	}
	if err := TransitionTaskToDone(&idx, "task-1"); err != nil {
		t.Fatalf("transition to done: %v", err)
	}

	if got := idx.Tasks[0].State; got != TaskStateDone {
		t.Fatalf("expected final state %q, got %q", TaskStateDone, got)
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
				State: TaskStateOpen,
				Role:  "builder",
			},
		},
	}

	if err := TransitionTaskToWorked(&idx, "task-1"); err != nil {
		t.Fatalf("transition to worked: %v", err)
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
	if err := TransitionTaskToDone(&idx, "task-1"); err != nil {
		t.Fatalf("transition to done: %v", err)
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

	if err := TransitionTaskToOpen(&idx, "task-1"); err != nil {
		t.Fatalf("transition to open: %v", err)
	}
	if got := idx.Tasks[0].State; got != TaskStateOpen {
		t.Fatalf("expected state %q, got %q", TaskStateOpen, got)
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
				State: TaskStateOpen,
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

// TestTransitionFromDoneToWorkedFails rejects invalid transitions.
func TestTransitionFromDoneToWorkedFails(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateDone,
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

	err := TransitionTaskToWorked(&idx, "task-1")
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
