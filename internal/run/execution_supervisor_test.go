// Tests for execution supervisor completion cleanup.
package run

import (
	"os"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestRunExecutionSupervisorClearsStateOnCompletion verifies completed supervisors do not persist state.
func TestRunExecutionSupervisorClearsStateOnCompletion(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repoRoot := repo.Root

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}
	if err := SeedPlanningIndex(repoRoot); err != nil {
		t.Fatalf("seed planning index: %v", err)
	}

	tasks := []index.Task{
		{ID: "T-EXEC-001", Kind: index.TaskKindExecution, State: index.TaskStateMerged},
	}
	writeTestTaskIndex(t, repoRoot, tasks)

	if err := RunExecutionSupervisor(repoRoot, ExecutionSupervisorOptions{PollInterval: 10 * time.Millisecond}); err != nil {
		t.Fatalf("run execution supervisor: %v", err)
	}

	if _, err := os.Stat(supervisor.ExecutionStatePath(repoRoot)); !os.IsNotExist(err) {
		t.Fatalf("execution supervisor state should be cleared, stat err=%v", err)
	}
}
