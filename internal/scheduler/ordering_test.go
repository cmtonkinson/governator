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
			{ID: "task-done", Kind: index.TaskKindExecution, State: index.TaskStateDone, Order: 0},
			{ID: "task-tested-a", Kind: index.TaskKindExecution, State: index.TaskStateTested, Order: 2, Dependencies: []string{"task-done"}},
			{ID: "task-tested-b", Kind: index.TaskKindExecution, State: index.TaskStateTested, Order: 2, Dependencies: []string{"task-done"}},
			{ID: "task-worked", Kind: index.TaskKindExecution, State: index.TaskStateWorked, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-open-early", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-open-late", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 3, Dependencies: []string{"task-done"}},
			{ID: "task-blocked", Kind: index.TaskKindExecution, State: index.TaskStateBlocked, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-open-needs-open", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 4, Dependencies: []string{"task-open-early"}},
		},
	}

	ordered, err := OrderedEligibleTasks(idx, nil)
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
			{ID: "task-a", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Dependencies: []string{"task-b"}},
			{ID: "task-b", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Dependencies: []string{"task-c"}},
			{ID: "task-c", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Dependencies: []string{"task-a"}},
		},
	}

	_, err := OrderedEligibleTasks(idx, nil)
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
			{ID: "task-done", Kind: index.TaskKindExecution, State: index.TaskStateDone},
			{ID: "task-conflict", Kind: index.TaskKindExecution, State: index.TaskStateConflict, Order: 2, Dependencies: []string{"task-done"}},
			{ID: "task-resolved", Kind: index.TaskKindExecution, State: index.TaskStateResolved, Order: 1, Dependencies: []string{"task-done"}},
			{ID: "task-tested", Kind: index.TaskKindExecution, State: index.TaskStateTested, Order: 0, Dependencies: []string{"task-done"}},
			{ID: "task-worked", Kind: index.TaskKindExecution, State: index.TaskStateWorked, Order: 0, Dependencies: []string{"task-done"}},
			{ID: "task-open", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 0, Dependencies: []string{"task-done"}},
		},
	}

	ordered, err := OrderedEligibleTasks(idx, nil)
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

// TestOrderedEligibleTasksSkipsInFlight ensures in-flight tasks are excluded.
func TestOrderedEligibleTasksSkipsInFlight(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-a", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 1},
			{ID: "task-b", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 2},
		},
	}

	inFlight := map[string]struct{}{"task-a": {}}
	ordered, err := OrderedEligibleTasks(idx, inFlight)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(ordered) != 1 {
		t.Fatalf("got %d tasks, want 1", len(ordered))
	}
	if ordered[0].ID != "task-b" {
		t.Fatalf("ordered[0] = %s, want task-b", ordered[0].ID)
	}
}

// TestOrderedEligibleTasksRespectsDependencies verifies tasks are only eligible when their dependencies are done.
func TestOrderedEligibleTasksRespectsDependencies(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-base", Kind: index.TaskKindExecution, State: index.TaskStateDone},
			{ID: "task-ready", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 2, Dependencies: []string{"task-base"}},
			{ID: "task-no-deps", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 1},
			{ID: "task-blocked", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Order: 3, Dependencies: []string{"task-missing"}},
		},
	}

	ordered, err := OrderedEligibleTasks(idx, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"task-no-deps", "task-ready"}
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
