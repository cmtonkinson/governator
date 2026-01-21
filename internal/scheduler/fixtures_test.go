package scheduler

import (
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestSchedulerOrderingHonorsDependencyChainAndOverlap uses the dependency chain fixture to verify eligibility and overlap reasoning.
func TestSchedulerOrderingHonorsDependencyChainAndOverlap(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repo.ApplyFixture(t, "dependency-chain-overlap")

	indexPath := filepath.Join(repo.Root, "_governator", "plan", "task-index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load fixture index: %v", err)
	}

	ordered, err := OrderedEligibleTasks(idx, nil)
	if err != nil {
		t.Fatalf("OrderedEligibleTasks: %v", err)
	}

	var orderedIDs []string
	for _, task := range ordered {
		orderedIDs = append(orderedIDs, task.ID)
	}

	expectedIDs := []string{"T-202", "T-204", "T-205"}
	if len(orderedIDs) != len(expectedIDs) {
		t.Fatalf("ordered IDs = %v, want %v", orderedIDs, expectedIDs)
	}
	for i, id := range expectedIDs {
		if orderedIDs[i] != id {
			t.Fatalf("ordered[%d] = %s, want %s", i, orderedIDs[i], id)
		}
	}

	result := RouteOrderedTasks(ordered, RoleCaps{Global: 3, DefaultRole: 3})
	if len(result.Selected) != 2 {
		t.Fatalf("selected tasks = %d, want 2", len(result.Selected))
	}
	if result.Selected[0].ID != "T-202" || result.Selected[1].ID != "T-204" {
		t.Fatalf("selected IDs = %v, want %v", []string{result.Selected[0].ID, result.Selected[1].ID}, []string{"T-202", "T-204"})
	}

	var overlapDecision *RoutingDecision
	for i := range result.Decisions {
		decision := &result.Decisions[i]
		if decision.Task.ID == "T-205" {
			overlapDecision = decision
			break
		}
	}
	if overlapDecision == nil {
		t.Fatalf("missing decision for T-205")
	}
	if overlapDecision.Selected {
		t.Fatalf("unexpectedly selected T-205 despite overlap conflict")
	}
	if overlapDecision.Reason != reasonOverlapConflict {
		t.Fatalf("reason = %q, want %q", overlapDecision.Reason, reasonOverlapConflict)
	}
}
