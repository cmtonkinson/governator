// Tests for supervisor state persistence and process checks.
package supervisor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatePath(t *testing.T) {
	repoRoot := "/tmp/testrepo"
	expectedPlanning := filepath.Join(repoRoot, "_governator/_local-state/planning_supervisor/state.json")
	expectedExecution := filepath.Join(repoRoot, "_governator/_local-state/execution_supervisor/state.json")

	if got := StatePath(repoRoot, SupervisorKindPlanning); got != expectedPlanning {
		t.Errorf("StatePath(Planning) got %s, want %s", got, expectedPlanning)
	}
	if got := StatePath(repoRoot, SupervisorKindExecution); got != expectedExecution {
		t.Errorf("StatePath(Execution) got %s, want %s", got, expectedExecution)
	}
}

func TestLogPath(t *testing.T) {
	repoRoot := "/tmp/testrepo"
	expectedPlanning := filepath.Join(repoRoot, "_governator/_local-state/planning_supervisor/supervisor.log")
	expectedExecution := filepath.Join(repoRoot, "_governator/_local-state/execution_supervisor/supervisor.log")

	if got := LogPath(repoRoot, SupervisorKindPlanning); got != expectedPlanning {
		t.Errorf("LogPath(Planning) got %s, want %s", got, expectedPlanning)
	}
	if got := LogPath(repoRoot, SupervisorKindExecution); got != expectedExecution {
		t.Errorf("LogPath(Execution) got %s, want %s", got, expectedExecution)
	}
}

func TestSaveLoadClearState(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := tmpDir

	testState := SupervisorStateInfo{
		Phase:          "test-phase",
		PID:            12345,
		State:          SupervisorStateRunning,
		StartedAt:      time.Now().UTC().Truncate(time.Second),
		LastTransition: time.Now().UTC().Truncate(time.Second),
		LogPath:        LogPath(repoRoot, SupervisorKindPlanning),
	}

	// Test SaveState for Planning
	err := SaveState(repoRoot, SupervisorKindPlanning, testState)
	if err != nil {
		t.Fatalf("SaveState(Planning) failed: %v", err)
	}

	// Test LoadState for Planning
	loadedState, found, err := LoadState(repoRoot, SupervisorKindPlanning)
	if err != nil {
		t.Fatalf("LoadState(Planning) failed: %v", err)
	}
	if !found {
		t.Fatalf("LoadState(Planning) did not find state")
	}
	if loadedState != testState {
		t.Errorf("Loaded Planning state mismatch. Got %+v, want %+v", loadedState, testState)
	}

	// Test type aliases and wrappers
	planningStateAlias, foundAlias, err := LoadPlanningState(repoRoot)
	if err != nil {
		t.Fatalf("LoadPlanningState failed: %v", err)
	}
	if !foundAlias {
		t.Fatalf("LoadPlanningState did not find state")
	}
	if PlanningSupervisorState(loadedState) != planningStateAlias {
		t.Errorf("PlanningSupervisorState alias mismatch. Got %+v, want %+v", planningStateAlias, PlanningSupervisorState(loadedState))
	}

	// Test ClearState for Planning
	err = ClearState(repoRoot, SupervisorKindPlanning)
	if err != nil {
		t.Fatalf("ClearState(Planning) failed: %v", err)
	}
	_, found, err = LoadState(repoRoot, SupervisorKindPlanning)
	if err != nil {
		t.Fatalf("LoadState(Planning) after clear failed: %v", err)
	}
	if found {
		t.Fatalf("LoadState(Planning) found state after clear")
	}

	// Test SaveState for Execution
	err = SaveState(repoRoot, SupervisorKindExecution, testState)
	if err != nil {
		t.Fatalf("SaveState(Execution) failed: %v", err)
	}

	// Test LoadState for Execution
	loadedState, found, err = LoadState(repoRoot, SupervisorKindExecution)
	if err != nil {
		t.Fatalf("LoadState(Execution) failed: %v", err)
	}
	if !found {
		t.Fatalf("LoadState(Execution) did not find state")
	}
	if loadedState != testState {
		t.Errorf("Loaded Execution state mismatch. Got %+v, want %+v", loadedState, testState)
	}

	// Test ClearState for Execution
	err = ClearState(repoRoot, SupervisorKindExecution)
	if err != nil {
		t.Fatalf("ClearState(Execution) failed: %v", err)
	}
	_, found, err = LoadState(repoRoot, SupervisorKindExecution)
	if err != nil {
		t.Fatalf("LoadState(Execution) after clear failed: %v", err)
	}
	if found {
		t.Fatalf("LoadState(Execution) found state after clear")
	}
}

func TestSupervisorRunning(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := tmpDir

	// Ensure no supervisor is initially running
	_, running, err := SupervisorRunning(repoRoot, SupervisorKindPlanning)
	if err != nil {
		t.Fatalf("SupervisorRunning(Planning) failed: %v", err)
	}
	if running {
		t.Fatalf("SupervisorRunning(Planning) reported running prematurely")
	}

	// Test with a dummy PID that exists (current process)
	testState := SupervisorStateInfo{
		PID:   os.Getpid(),
		State: SupervisorStateRunning,
	}
	err = SaveState(repoRoot, SupervisorKindPlanning, testState)
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	state, running, err := SupervisorRunning(repoRoot, SupervisorKindPlanning)
	if err != nil {
		t.Fatalf("SupervisorRunning(Planning) failed: %v", err)
	}
	if !running {
		t.Fatalf("SupervisorRunning(Planning) reported not running when it should be")
	}
	if state.PID != os.Getpid() {
		t.Errorf("SupervisorRunning PID mismatch. Got %d, want %d", state.PID, os.Getpid())
	}

	// Test with a non-existent PID
	testState.PID = 999999 // Hopefully a non-existent PID
	err = SaveState(repoRoot, SupervisorKindPlanning, testState)
	if err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	_, running, err = SupervisorRunning(repoRoot, SupervisorKindPlanning)
	if err != nil {
		t.Fatalf("SupervisorRunning(Planning) with bad PID failed: %v", err)
	}
	if running {
		t.Fatalf("SupervisorRunning(Planning) with bad PID reported running")
	}

	// Test wrappers
	planningStateAlias, runningAlias, err := PlanningSupervisorRunning(repoRoot)
	if err != nil {
		t.Fatalf("PlanningSupervisorRunning failed: %v", err)
	}
	if runningAlias {
		t.Fatalf("PlanningSupervisorRunning with bad PID reported running")
	}
	if PlanningSupervisorState(testState) != planningStateAlias {
		t.Errorf("PlanningSupervisorRunning alias mismatch. Got %+v, want %+v", planningStateAlias, PlanningSupervisorState(testState))
	}
}

func TestAnySupervisorRunning(t *testing.T) {
	tmpDir := t.TempDir()
	repoRoot := tmpDir

	// No supervisors running
	kind, running, err := AnySupervisorRunning(repoRoot)
	if err != nil {
		t.Fatalf("AnySupervisorRunning (none) failed: %v", err)
	}
	if running {
		t.Fatalf("AnySupervisorRunning (none) reported running")
	}

	// Planning supervisor running
	planningState := SupervisorStateInfo{PID: os.Getpid(), State: SupervisorStateRunning, Phase: "planning"}
	SaveState(repoRoot, SupervisorKindPlanning, planningState)
	kind, running, err = AnySupervisorRunning(repoRoot)
	if err != nil {
		t.Fatalf("AnySupervisorRunning (planning) failed: %v", err)
	}
	if !running || kind != string(SupervisorKindPlanning) {
		t.Fatalf("AnySupervisorRunning (planning) reported %s, running %t, want planning, true", kind, running)
	}
	ClearState(repoRoot, SupervisorKindPlanning)

	// Execution supervisor running
	execState := SupervisorStateInfo{PID: os.Getpid(), State: SupervisorStateRunning, Phase: "execution"}
	SaveState(repoRoot, SupervisorKindExecution, execState)
	kind, running, err = AnySupervisorRunning(repoRoot)
	if err != nil {
		t.Fatalf("AnySupervisorRunning (execution) failed: %v", err)
	}
	if !running || kind != string(SupervisorKindExecution) {
		t.Fatalf("AnySupervisorRunning (execution) reported %s, running %t, want execution, true", kind, running)
	}
	ClearState(repoRoot, SupervisorKindExecution)

	// Both supervisors running (execution should be preferred)
	SaveState(repoRoot, SupervisorKindPlanning, planningState)
	SaveState(repoRoot, SupervisorKindExecution, execState)
	kind, running, err = AnySupervisorRunning(repoRoot)
	if err != nil {
		t.Fatalf("AnySupervisorRunning (both) failed: %v", err)
	}
	if !running || kind != string(SupervisorKindExecution) {
		t.Fatalf("AnySupervisorRunning (both) reported %s, running %t, want execution, true", kind, running)
	}
	ClearState(repoRoot, SupervisorKindPlanning)
	ClearState(repoRoot, SupervisorKindExecution)
}
