// Package scheduler provides tests for role cap enforcement.
package scheduler

import (
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
)

// TestApplyRoleCapsExample verifies the routing example from the policy doc.
func TestApplyRoleCapsExample(t *testing.T) {
	ordered := []index.Task{
		{ID: "T-01", Role: "resolver"},
		{ID: "T-02", Role: "worker"},
		{ID: "T-03", Role: "worker"},
		{ID: "T-04", Role: "worker"},
		{ID: "T-05", Role: "worker"},
		{ID: "T-06", Role: "tester"},
		{ID: "T-07", Role: "tester"},
	}
	caps := RoleCaps{
		Global:      5,
		DefaultRole: 3,
		Roles: map[index.Role]int{
			"reviewer": 1,
		},
	}

	selected := ApplyRoleCaps(ordered, caps)

	want := []string{"T-01", "T-02", "T-03", "T-04", "T-06"}
	got := taskIDs(selected)
	if len(got) != len(want) {
		t.Fatalf("selected %d tasks, want %d", len(got), len(want))
	}
	for i, id := range want {
		if got[i] != id {
			t.Fatalf("selected[%d] = %s, want %s", i, got[i], id)
		}
	}
}

// TestApplyRoleCapsUsesDefaultRoleCap ensures missing per-role caps use the default.
func TestApplyRoleCapsUsesDefaultRoleCap(t *testing.T) {
	ordered := []index.Task{
		{ID: "T-01", Role: "planner"},
		{ID: "T-02", Role: "planner"},
		{ID: "T-03", Role: "planner"},
	}
	caps := RoleCaps{
		Global:      10,
		DefaultRole: 2,
		Roles:       map[index.Role]int{},
	}

	selected := ApplyRoleCaps(ordered, caps)

	if len(selected) != 2 {
		t.Fatalf("selected %d tasks, want 2", len(selected))
	}
}

// TestRoleCapsFromConfigDefaults ensures invalid caps fall back to defaults.
func TestRoleCapsFromConfigDefaults(t *testing.T) {
	defaults := config.Defaults()
	cfg := config.Config{
		Concurrency: config.ConcurrencyConfig{
			Global:      0,
			DefaultRole: 0,
			Roles: map[string]int{
				"planner":  -1,
				"executor": 3,
			},
		},
	}

	caps := RoleCapsFromConfig(cfg)

	if caps.Global != defaults.Concurrency.Global {
		t.Fatalf("global cap = %d, want %d", caps.Global, defaults.Concurrency.Global)
	}
	if caps.DefaultRole != defaults.Concurrency.DefaultRole {
		t.Fatalf("default role cap = %d, want %d", caps.DefaultRole, defaults.Concurrency.DefaultRole)
	}
	if _, ok := caps.Roles["planner"]; ok {
		t.Fatal("planner cap should be filtered out")
	}
	if caps.Roles["executor"] != 3 {
		t.Fatalf("executor cap = %d, want 3", caps.Roles["executor"])
	}
}

// taskIDs extracts task IDs for comparison.
func taskIDs(tasks []index.Task) []string {
	ids := make([]string, 0, len(tasks))
	for _, task := range tasks {
		ids = append(ids, task.ID)
	}
	return ids
}
