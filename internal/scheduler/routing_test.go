// Package scheduler provides tests for routing decisions and selection.
package scheduler

import (
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

// TestRouteEligibleTasksStageBias prefers review/test stages before open work.
func TestRouteEligibleTasksStageBias(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "review-1", State: index.TaskStateTested, Role: "reviewer", Order: 10},
			{ID: "review-2", State: index.TaskStateTested, Role: "reviewer", Order: 20},
			{ID: "review-3", State: index.TaskStateTested, Role: "reviewer", Order: 30},
			{ID: "test-1", State: index.TaskStateWorked, Role: "tester", Order: 10},
			{ID: "test-2", State: index.TaskStateWorked, Role: "tester", Order: 20},
			{ID: "test-3", State: index.TaskStateWorked, Role: "tester", Order: 30},
			{ID: "open-1", State: index.TaskStateOpen, Role: "worker", Order: 10},
		},
	}
	caps := RoleCaps{
		Global:      5,
		DefaultRole: 3,
	}

	result, err := RouteEligibleTasks(idx, caps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"review-1", "review-2", "review-3", "test-1", "test-2"}
	got := taskIDs(result.Selected)
	if len(got) != len(want) {
		t.Fatalf("selected %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("selected[%d] = %s, want %s", i, got[i], id)
		}
	}

	for _, decision := range result.Decisions {
		if !decision.Selected {
			t.Fatalf("unexpected skipped decision for %s: %s", decision.Task.ID, decision.Reason)
		}
		if decision.Reason != reasonSelected {
			t.Fatalf("decision for %s = %q, want %q", decision.Task.ID, decision.Reason, reasonSelected)
		}
	}
}

// TestRouteEligibleTasksOpenFallback selects open tasks when no review/test tasks exist.
func TestRouteEligibleTasksOpenFallback(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "open-1", State: index.TaskStateOpen, Role: "worker", Order: 10},
			{ID: "open-2", State: index.TaskStateOpen, Role: "worker", Order: 20},
			{ID: "open-3", State: index.TaskStateOpen, Role: "worker", Order: 30},
		},
	}
	caps := RoleCaps{
		Global:      2,
		DefaultRole: 3,
	}

	result, err := RouteEligibleTasks(idx, caps)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"open-1", "open-2"}
	got := taskIDs(result.Selected)
	if len(got) != len(want) {
		t.Fatalf("selected %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("selected[%d] = %s, want %s", i, got[i], id)
		}
	}

	for _, decision := range result.Decisions {
		if !decision.Selected {
			t.Fatalf("unexpected skipped decision for %s: %s", decision.Task.ID, decision.Reason)
		}
		if decision.Reason != reasonSelected {
			t.Fatalf("decision for %s = %q, want %q", decision.Task.ID, decision.Reason, reasonSelected)
		}
	}
}
