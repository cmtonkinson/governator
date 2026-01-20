// Package scheduler provides tests for scheduler ordering behavior.
package scheduler

import (
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

// TestOrderedEligibleTasksSimpleDAG ensures eligible tasks are ordered deterministically.
func TestOrderedEligibleTasksSimpleDAG(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-done", State: index.TaskStateDone, Order: 0},
			{ID: "task-tested-a", State: index.TaskStateTested, Order: 2, Dependencies: []string{"task-done"}},
			{ID: "task-tested-b", State: index.TaskStateTested, Order: 2, Dependencies: []string{"task-done"}},
			{ID: "task-worked", State: index.TaskStateWorked, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-open-early", State: index.TaskStateOpen, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-open-late", State: index.TaskStateOpen, Order: 3, Dependencies: []string{"task-done"}},
			{ID: "task-blocked", State: index.TaskStateBlocked, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-open-needs-open", State: index.TaskStateOpen, Order: 4, Dependencies: []string{"task-open-early"}},
		},
	}

	ordered, err := OrderedEligibleTasks(idx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"task-tested-a",
		"task-tested-b",
		"task-worked",
		"task-open-early",
		"task-open-late",
	}
	got := taskIDs(ordered)
	if len(got) != len(want) {
		t.Fatalf("got %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("ordered[%d] = %s, want %s", i, got[i], id)
		}
	}
}

// TestOrderedEligibleTasksDetectsCycles ensures circular dependencies are reported.
func TestOrderedEligibleTasksDetectsCycles(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-a", State: index.TaskStateOpen, Dependencies: []string{"task-b"}},
			{ID: "task-b", State: index.TaskStateOpen, Dependencies: []string{"task-c"}},
			{ID: "task-c", State: index.TaskStateOpen, Dependencies: []string{"task-a"}},
		},
	}

	_, err := OrderedEligibleTasks(idx)
	if err == nil {
		t.Fatal("expected cycle error, got nil")
	}
	if !strings.Contains(err.Error(), "circular dependency") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestOrderedEligibleTasksConflictPriority ensures conflict states are routed first.
func TestOrderedEligibleTasksConflictPriority(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-done", State: index.TaskStateDone},
			{ID: "task-conflict", State: index.TaskStateConflict, Order: 2, Dependencies: []string{"task-done"}},
			{ID: "task-resolved", State: index.TaskStateResolved, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-tested", State: index.TaskStateTested, Order: 0, Dependencies: []string{"task-done"}},
			{ID: "task-worked", State: index.TaskStateWorked, Order: 0, Dependencies: []string{"task-done"}},
			{ID: "task-open", State: index.TaskStateOpen, Order: 0, Dependencies: []string{"task-done"}},
		},
	}

	ordered, err := OrderedEligibleTasks(idx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{
		"task-resolved",
		"task-conflict",
		"task-tested",
		"task-worked",
		"task-open",
	}
	got := taskIDs(ordered)
	if len(got) != len(want) {
		t.Fatalf("got %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("ordered[%d] = %s, want %s", i, got[i], id)
		}
	}
}
