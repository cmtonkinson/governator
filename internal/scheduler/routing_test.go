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
			{ID: "review-1", Kind: index.TaskKindExecution, State: index.TaskStateTested, Role: "reviewer", Order: 10},
			{ID: "review-2", Kind: index.TaskKindExecution, State: index.TaskStateTested, Role: "reviewer", Order: 20},
			{ID: "review-3", Kind: index.TaskKindExecution, State: index.TaskStateTested, Role: "reviewer", Order: 30},
			{ID: "test-1", Kind: index.TaskKindExecution, State: index.TaskStateWorked, Role: "tester", Order: 10},
			{ID: "test-2", Kind: index.TaskKindExecution, State: index.TaskStateWorked, Role: "tester", Order: 20},
			{ID: "test-3", Kind: index.TaskKindExecution, State: index.TaskStateWorked, Role: "tester", Order: 30},
			{ID: "open-1", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 10},
		},
	}
	caps := RoleCaps{
		Global:      5,
		DefaultRole: 3,
	}

	result, err := RouteEligibleTasks(idx, caps, nil)
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
			{ID: "open-1", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 10},
			{ID: "open-2", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 20},
			{ID: "open-3", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 30},
		},
	}
	caps := RoleCaps{
		Global:      2,
		DefaultRole: 3,
	}

	result, err := RouteEligibleTasks(idx, caps, nil)
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

// TestRouteEligibleTasksOverlapAllowsParallel selects non-overlapping tasks together.
func TestRouteEligibleTasksOverlapAllowsParallel(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-a", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 10, Overlap: []string{"db"}},
			{ID: "task-b", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 20, Overlap: []string{"api"}},
		},
	}
	caps := RoleCaps{
		Global:      2,
		DefaultRole: 2,
	}

	result, err := RouteEligibleTasks(idx, caps, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"task-a", "task-b"}
	got := taskIDs(result.Selected)
	if len(got) != len(want) {
		t.Fatalf("selected %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("selected[%d] = %s, want %s", i, got[i], id)
		}
	}
}

// TestRouteEligibleTasksOverlapSerializes ensures overlapping tasks are not scheduled together.
func TestRouteEligibleTasksOverlapSerializes(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-a", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 10, Overlap: []string{"db"}},
			{ID: "task-b", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 20, Overlap: []string{"db"}},
			{ID: "task-c", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 30, Overlap: []string{"api"}},
		},
	}
	caps := RoleCaps{
		Global:      2,
		DefaultRole: 2,
	}

	result, err := RouteEligibleTasks(idx, caps, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"task-a", "task-c"}
	got := taskIDs(result.Selected)
	if len(got) != len(want) {
		t.Fatalf("selected %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("selected[%d] = %s, want %s", i, got[i], id)
		}
	}

	foundOverlapSkip := false
	for _, decision := range result.Decisions {
		if decision.Task.ID == "task-b" {
			foundOverlapSkip = true
			if decision.Selected {
				t.Fatalf("expected task-b to be skipped")
			}
			if decision.Reason != reasonOverlapConflict {
				t.Fatalf("decision for task-b = %q, want %q", decision.Reason, reasonOverlapConflict)
			}
		}
	}
	if !foundOverlapSkip {
		t.Fatalf("expected overlap conflict decision for task-b")
	}
}

// TestRouteEligibleTasksOverlapAcrossStages blocks overlap across lifecycle stages.
func TestRouteEligibleTasksOverlapAcrossStages(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "review-1", Kind: index.TaskKindExecution, State: index.TaskStateTested, Role: "reviewer", Order: 10, Overlap: []string{"db"}},
			{ID: "open-1", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 10, Overlap: []string{"db"}},
			{ID: "open-2", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker", Order: 20, Overlap: []string{"api"}},
		},
	}
	caps := RoleCaps{
		Global:      2,
		DefaultRole: 2,
	}

	result, err := RouteEligibleTasks(idx, caps, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := []string{"review-1", "open-2"}
	got := taskIDs(result.Selected)
	if len(got) != len(want) {
		t.Fatalf("selected %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("selected[%d] = %s, want %s", i, got[i], id)
		}
	}
}

// TestRouteEligibleTasksReasonRoleCapReached ensures decisions report skipping tasks when a role cap is reached.
func TestRouteEligibleTasksReasonRoleCapReached(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "task-1", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker"},
			{ID: "task-2", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker"},
			{ID: "task-3", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "worker"},
		},
	}
	caps := RoleCaps{
		Global:      3,
		DefaultRole: 1,
	}

	result, err := RouteEligibleTasks(idx, caps, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Selected) != 1 || result.Selected[0].ID != "task-1" {
		t.Fatalf("selected tasks = %v, want [task-1]", taskIDs(result.Selected))
	}

	reachedCount := 0
	for _, decision := range result.Decisions {
		if decision.Task.ID == "task-2" || decision.Task.ID == "task-3" {
			if decision.Selected {
				t.Fatalf("expected %s to be skipped", decision.Task.ID)
			}
			if decision.Reason != reasonRoleCapReached {
				t.Fatalf("decision for %s = %q, want %q", decision.Task.ID, decision.Reason, reasonRoleCapReached)
			}
			reachedCount++
		}
	}

	if reachedCount != 2 {
		t.Fatalf("got %d role-cap decisions, want 2", reachedCount)
	}
}

// TestRouteEligibleTasksReasonRoleCapDisabled ensures decisions report when a role cap is disabled.
func TestRouteEligibleTasksReasonRoleCapDisabled(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{ID: "disabled-1", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "disabled"},
			{ID: "enabled-1", Kind: index.TaskKindExecution, State: index.TaskStateOpen, Role: "enabled"},
		},
	}
	caps := RoleCaps{
		Global:      2,
		DefaultRole: 2,
		Roles: map[index.Role]int{
			"disabled": 0,
		},
	}

	result, err := RouteEligibleTasks(idx, caps, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Selected) != 1 || result.Selected[0].ID != "enabled-1" {
		t.Fatalf("selected tasks = %v, want [enabled-1]", taskIDs(result.Selected))
	}

	disabledDecisionFound := false
	for _, decision := range result.Decisions {
		if decision.Task.ID == "disabled-1" {
			disabledDecisionFound = true
			if decision.Selected {
				t.Fatalf("expected disabled-1 to be skipped")
			}
			if decision.Reason != reasonRoleCapDisabled {
				t.Fatalf("decision for disabled-1 = %q, want %q", decision.Reason, reasonRoleCapDisabled)
			}
		}
	}

	if !disabledDecisionFound {
		t.Fatalf("expected a decision for disabled-1")
	}
}
