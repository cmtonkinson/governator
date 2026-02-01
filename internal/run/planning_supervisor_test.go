// Tests for planning supervisor completion cleanup.
package run

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestRunPlanningSupervisorClearsStateOnCompletion verifies completed supervisors do not persist state.
func TestRunPlanningSupervisorClearsStateOnCompletion(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repoRoot := repo.Root

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}
	if err := SeedPlanningIndex(repoRoot); err != nil {
		t.Fatalf("seed planning index: %v", err)
	}

	indexPath := filepath.Join(repoRoot, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load task index: %v", err)
	}
	updatePlanningTaskState(&idx, "")
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save task index: %v", err)
	}

	if err := RunPlanningSupervisor(repoRoot, PlanningSupervisorOptions{PollInterval: 10 * time.Millisecond}); err != nil {
		t.Fatalf("run planning supervisor: %v", err)
	}

	if _, err := os.Stat(supervisor.PlanningStatePath(repoRoot)); !os.IsNotExist(err) {
		t.Fatalf("planning supervisor state should be cleared, stat err=%v", err)
	}
}
