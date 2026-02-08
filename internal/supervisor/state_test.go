// Tests for unified supervisor state persistence.
package supervisor

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPaths(t *testing.T) {
	repoRoot := "/tmp/testrepo"
	expectedState := filepath.Join(repoRoot, "_governator/_local-state/supervisor/state.json")
	expectedLog := filepath.Join(repoRoot, "_governator/_local-state/supervisor/supervisor.log")

	if got := StatePath(repoRoot); got != expectedState {
		t.Fatalf("StatePath got %s, want %s", got, expectedState)
	}
	if got := LogPath(repoRoot); got != expectedLog {
		t.Fatalf("LogPath got %s, want %s", got, expectedLog)
	}
}

func TestStatePersistence(t *testing.T) {
	repoRoot := t.TempDir()
	testState := SupervisorStateInfo{
		Phase:          "test",
		PID:            os.Getpid(),
		State:          SupervisorStateRunning,
		StartedAt:      time.Now().UTC().Truncate(time.Second),
		LastTransition: time.Now().UTC().Truncate(time.Second),
		LogPath:        LogPath(repoRoot),
	}

	if err := SaveState(repoRoot, testState); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, found, err := LoadState(repoRoot)
	if err != nil {
		t.Fatalf("LoadState failed: %v", err)
	}
	if !found {
		t.Fatalf("LoadState did not find state")
	}
	if loaded != testState {
		t.Fatalf("Loaded state mismatch. Got %+v, want %+v", loaded, testState)
	}

	if err := ClearState(repoRoot); err != nil {
		t.Fatalf("ClearState failed: %v", err)
	}
	_, found, err = LoadState(repoRoot)
	if err != nil {
		t.Fatalf("LoadState after clear failed: %v", err)
	}
	if found {
		t.Fatalf("LoadState found state after clear")
	}
}

func TestSupervisorRunning(t *testing.T) {
	repoRoot := t.TempDir()

	state := SupervisorStateInfo{PID: os.Getpid(), State: SupervisorStateRunning}
	if err := SaveState(repoRoot, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	loaded, running, err := SupervisorRunning(repoRoot)
	if err != nil {
		t.Fatalf("SupervisorRunning failed: %v", err)
	}
	if !running {
		t.Fatal("SupervisorRunning reported not running")
	}
	if loaded.PID != os.Getpid() {
		t.Fatalf("SupervisorRunning PID mismatch: got %d, want %d", loaded.PID, os.Getpid())
	}

	state.PID = 999999
	if err := SaveState(repoRoot, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}

	_, running, err = SupervisorRunning(repoRoot)
	if err != nil {
		t.Fatalf("SupervisorRunning with bad PID failed: %v", err)
	}
	if running {
		t.Fatal("SupervisorRunning reported running for bad PID")
	}
}

func TestAnyRunning(t *testing.T) {
	repoRoot := t.TempDir()

	kind, running, err := AnyRunning(repoRoot)
	if err != nil {
		t.Fatalf("AnyRunning failed: %v", err)
	}
	if running {
		t.Fatalf("AnyRunning reported running when none exist")
	}

	state := SupervisorStateInfo{PID: os.Getpid(), State: SupervisorStateRunning}
	if err := SaveState(repoRoot, state); err != nil {
		t.Fatalf("SaveState failed: %v", err)
	}
	defer func() {
		_ = ClearState(repoRoot)
	}()

	kind, running, err = AnyRunning(repoRoot)
	if err != nil {
		t.Fatalf("AnyRunning failed: %v", err)
	}
	if !running || kind != "supervisor" {
		t.Fatalf("AnyRunning reported %s/%t, want supervisor/true", kind, running)
	}
}
