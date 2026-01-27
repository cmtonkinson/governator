// Tests for planning phase in-flight tracking and collection.
package run

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/phase"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/testrepos"
	"github.com/cmtonkinson/governator/internal/worker"
)

// TestPhaseRunnerPlanningInFlight verifies planning uses the shared in-flight store.
func TestPhaseRunnerPlanningInFlight(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repoRoot := repo.Root
	setupPlanningRepo(t, repoRoot, repo)

	stateStore := phase.NewStore(repoRoot)
	state := phase.DefaultState()
	if err := stateStore.Save(state); err != nil {
		t.Fatalf("save phase state: %v", err)
	}

	cfg, err := config.Load(repoRoot, nil, nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("new in-flight store: %v", err)
	}
	inFlight := inflight.Set{}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	runner := newPhaseRunner(repoRoot, cfg, Options{Stdout: stdout, Stderr: stderr}, stateStore, inFlightStore, inFlight)

	archStep, ok := runner.planning.stepForPhase(phase.PhaseArchitectureBaseline)
	if !ok {
		t.Fatalf("missing architecture baseline step")
	}
	archID := archStep.workstreamID()

	handled, err := runner.EnsurePlanningPhases(&state)
	if err != nil {
		t.Fatalf("ensure planning phases (dispatch): %v", err)
	}
	if !handled {
		t.Fatalf("expected planning to be handled")
	}

	savedInFlight, err := inFlightStore.Load()
	if err != nil {
		t.Fatalf("load in-flight: %v", err)
	}
	archEntry, ok := savedInFlight.Entry(archID)
	if !ok {
		t.Fatalf("expected architecture step to be in-flight")
	}
	if archEntry.WorkerStateDir == "" {
		t.Fatalf("expected worker state dir for architecture step")
	}

	waitForPlanningExitStatus(t, archEntry.WorkerStateDir, archID, roles.StageWork)

	handled, err = runner.EnsurePlanningPhases(&state)
	if err != nil {
		t.Fatalf("ensure planning phases (collect): %v", err)
	}
	if !handled {
		t.Fatalf("expected planning to be handled on collect")
	}

	savedInFlight, err = inFlightStore.Load()
	if err != nil {
		t.Fatalf("reload in-flight: %v", err)
	}
	if savedInFlight.Contains(archID) {
		t.Fatalf("architecture step should be removed from in-flight after collection")
	}

	gapStep, ok := runner.planning.stepForPhase(phase.PhaseGapAnalysis)
	if !ok {
		t.Fatalf("missing gap analysis step")
	}
	if !savedInFlight.Contains(gapStep.workstreamID()) {
		t.Fatalf("expected gap analysis to be dispatched after architecture collection")
	}
}

// setupPlanningRepo ensures planning prompts, config, and the index are present and committed.
func setupPlanningRepo(t *testing.T, repoRoot string, repo *testrepos.TempRepo) {
	t.Helper()

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}
	writeRequiredDocs(t, repoRoot)
	if err := SeedPlanningIndex(repoRoot); err != nil {
		t.Fatalf("seed planning index: %v", err)
	}

	cfg := config.Defaults()
	command := []string{"sh", "-c", "true {task_path}"}
	cfg.Workers.Commands.Default = command
	cfg.Workers.Commands.Roles = map[string][]string{
		"architect": command,
		"default":   command,
		"planner":   command,
	}
	configPath := filepath.Join(repoRoot, "_governator", "_durable-state", "config.json")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	repo.RunGit(t, "add", "-A")
	repo.RunGit(t, "commit", "-m", "planning test setup")
}

// waitForPlanningExitStatus polls for exit.json written by the dispatch wrapper.
func waitForPlanningExitStatus(t *testing.T, workerStateDir string, taskID string, stage roles.Stage) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		_, found, err := worker.ReadExitStatus(workerStateDir, taskID, stage)
		if err != nil {
			t.Fatalf("read exit status: %v", err)
		}
		if found {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("exit status not found for %s", taskID)
}
