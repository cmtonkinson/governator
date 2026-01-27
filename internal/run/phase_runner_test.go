// Tests for the phase runner helper and its CLI gating behavior.
package run

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/bootstrap"
	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/phase"
)

func TestPhaseRunnerEnsurePhasePrereqsBlocksMissingArtifacts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := phase.NewStore(repoRoot)
	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("new in-flight store: %v", err)
	}
	stderr := &bytes.Buffer{}
	runner := newPhaseRunner(repoRoot, config.Defaults(), Options{Stdout: io.Discard, Stderr: stderr}, store, inFlightStore, inflight.Set{})

	err = runner.ensurePhasePrereqs(phase.PhaseGapAnalysis)
	if err == nil {
		t.Fatalf("expected gating error")
	}
	if !strings.Contains(err.Error(), "blocked") {
		t.Fatalf("unexpected error = %q", err.Error())
	}
	if !strings.Contains(stderr.String(), "phase gate") {
		t.Fatalf("expected phase gate log, got %q", stderr.String())
	}
}

func TestPhaseRunnerCompletePhaseAdvancesState(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	writeRequiredDocs(t, repoRoot)
	store := phase.NewStore(repoRoot)
	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("new in-flight store: %v", err)
	}
	state := phase.DefaultState()
	state.Current = phase.PhaseArchitectureBaseline
	state.LastCompleted = phase.PhaseNew
	stderr := &bytes.Buffer{}
	runner := newPhaseRunner(repoRoot, config.Defaults(), Options{Stdout: io.Discard, Stderr: stderr}, store, inFlightStore, inflight.Set{})

	if err := runner.completePhase(&state); err != nil {
		t.Fatalf("complete phase: %v", err)
	}

	if state.LastCompleted != phase.PhaseArchitectureBaseline {
		t.Fatalf("last completed = %v, want %v", state.LastCompleted, phase.PhaseArchitectureBaseline)
	}
	if state.Current != phase.PhaseGapAnalysis {
		t.Fatalf("current phase = %v, want %v", state.Current, phase.PhaseGapAnalysis)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr output: %q", stderr.String())
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load stored state: %v", err)
	}
	if loaded.Current != state.Current {
		t.Fatalf("stored current = %v, want %v", loaded.Current, state.Current)
	}
	if loaded.LastCompleted != state.LastCompleted {
		t.Fatalf("stored last completed = %v, want %v", loaded.LastCompleted, state.LastCompleted)
	}
}

func writeRequiredDocs(t *testing.T, repoRoot string) {
	t.Helper()
	docsDir := filepath.Join(repoRoot, "_governator", "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	for _, artifact := range bootstrap.Artifacts() {
		if !artifact.Required {
			continue
		}
		path := filepath.Join(docsDir, artifact.Name)
		if err := os.WriteFile(path, []byte("artifact"), 0o644); err != nil {
			t.Fatalf("write artifact %s: %v", artifact.Name, err)
		}
	}
}
