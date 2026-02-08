// Tests for the unified supervisor loop.
package run

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/supervisor"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

func TestRunUnifiedSupervisor_ClearsStateOnCompletion(t *testing.T) {
	repoRoot := setupUnifiedTestRepo(t)
	setPlanningState(t, repoRoot, PlanningCompleteState)

	prevRun := runFunc
	runFunc = func(repoRoot string, opts Options) (Result, error) {
		return Result{}, nil
	}
	t.Cleanup(func() { runFunc = prevRun })

	if err := RunUnifiedSupervisor(repoRoot, UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("RunUnifiedSupervisor failed: %v", err)
	}

	if _, ok, err := supervisor.LoadState(repoRoot); err != nil {
		t.Fatalf("load supervisor state: %v", err)
	} else if ok {
		t.Fatal("expected supervisor state to be cleared after completion")
	}
}

func TestRunUnifiedSupervisor_PlanningDriftDrainsAndReplans(t *testing.T) {
	repoRoot := setupUnifiedTestRepo(t)
	setPlanningState(t, repoRoot, PlanningCompleteState)

	adrDir := filepath.Join(repoRoot, "_governator/docs/adr")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("create adr dir: %v", err)
	}
	newADR := filepath.Join(adrDir, "adr-0001-test.md")
	if err := os.WriteFile(newADR, []byte("# ADR\n\nAdded for drift test.\n"), 0o644); err != nil {
		t.Fatalf("write ADR: %v", err)
	}

	prevRun := runFunc
	prevDetect := detectPlanningDriftFn
	driftChecks := 0
	sawGapAnalysis := false
	runFunc = func(repoRoot string, opts Options) (Result, error) {
		idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
		if err != nil {
			return Result{}, err
		}
		stateID, err := planningTaskState(idx)
		if err != nil {
			return Result{}, err
		}
		if stateID == "gap-analysis" {
			sawGapAnalysis = true
		}
		updatePlanningTaskState(&idx, PlanningCompleteState)
		if err := index.Save(filepath.Join(repoRoot, indexFilePath), idx); err != nil {
			return Result{}, err
		}
		return Result{}, nil
	}
	detectPlanningDriftFn = func(repoRoot string, base index.Digests) (PlanningDriftReport, error) {
		if driftChecks == 0 {
			driftChecks++
			return PlanningDriftReport{
				HasDrift: true,
				Details:  []string{"planning doc changed: _governator/docs/roadmap.md"},
				Message:  "planning drift detected",
			}, nil
		}
		return PlanningDriftReport{}, nil
	}
	t.Cleanup(func() {
		runFunc = prevRun
		detectPlanningDriftFn = prevDetect
	})

	if err := RunUnifiedSupervisor(repoRoot, UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("RunUnifiedSupervisor drift run failed: %v", err)
	}

	idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
	if err != nil {
		t.Fatalf("load index after drift run: %v", err)
	}
	stateID, err := planningTaskState(idx)
	if err != nil {
		t.Fatalf("planning task state: %v", err)
	}
	if !sawGapAnalysis {
		t.Fatal("expected planning state to reset to gap-analysis before replanning")
	}
	if stateID != PlanningCompleteState {
		t.Fatalf("expected planning state to complete after replanning, got %q", stateID)
	}
}

func TestRunUnifiedSupervisor_PlanningIncompleteRunsPlanning(t *testing.T) {
	repoRoot := setupUnifiedTestRepo(t)

	prevRun := runFunc
	called := false
	runFunc = func(repoRoot string, opts Options) (Result, error) {
		called = true
		idx, err := index.Load(filepath.Join(repoRoot, indexFilePath))
		if err != nil {
			return Result{}, err
		}
		updatePlanningTaskState(&idx, "")
		if err := index.Save(filepath.Join(repoRoot, indexFilePath), idx); err != nil {
			return Result{}, err
		}
		return Result{}, nil
	}
	t.Cleanup(func() { runFunc = prevRun })

	if err := RunUnifiedSupervisor(repoRoot, UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("RunUnifiedSupervisor planning run failed: %v", err)
	}
	if !called {
		t.Fatal("expected runFunc to be called when planning is incomplete")
	}
}

func setupUnifiedTestRepo(t *testing.T) string {
	t.Helper()
	repo := testrepos.New(t)
	repoRoot := repo.Root
	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init layout: %v", err)
	}
	writeTestPlanningSpec(t, repoRoot)
	if err := SeedPlanningIndex(repoRoot); err != nil {
		t.Fatalf("seed planning index: %v", err)
	}
	return repoRoot
}

func setPlanningState(t *testing.T, repoRoot string, state string) {
	t.Helper()
	idxPath := filepath.Join(repoRoot, indexFilePath)
	idx, err := index.Load(idxPath)
	if err != nil {
		t.Fatalf("load task index: %v", err)
	}
	updatePlanningTaskState(&idx, state)
	if err := index.Save(idxPath, idx); err != nil {
		t.Fatalf("save task index: %v", err)
	}
}
