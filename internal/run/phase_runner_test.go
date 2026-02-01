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
	writeTestPlanningSpec(t, repoRoot)
	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("new in-flight store: %v", err)
	}
	stderr := &bytes.Buffer{}
	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("load planning spec: %v", err)
	}
	runner := newPhaseRunner(repoRoot, config.Defaults(), Options{Stdout: io.Discard, Stderr: stderr}, inFlightStore, inflight.Set{}, planning)

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
	writeTestPlanningSpec(t, repoRoot)
	writeRequiredDocs(t, repoRoot)
	inFlightStore, err := inflight.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("new in-flight store: %v", err)
	}
	stderr := &bytes.Buffer{}
	planning, err := newPlanningTask(repoRoot)
	if err != nil {
		t.Fatalf("load planning spec: %v", err)
	}
	runner := newPhaseRunner(repoRoot, config.Defaults(), Options{Stdout: io.Discard, Stderr: stderr}, inFlightStore, inflight.Set{}, planning)
	step, ok := runner.planning.stepForPhase(phase.PhaseArchitectureBaseline)
	if !ok {
		t.Fatalf("missing architecture baseline step")
	}
	if err := runner.completePhase(step); err != nil {
		t.Fatalf("complete phase: %v", err)
	}

	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr output: %q", stderr.String())
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
	requiredPlanningDocs := map[string]string{
		"arch-asr.md":     "architecture ASR\n",
		"arch-arc42.md":   "architecture arc42\n",
		"gap-analysis.md": "gap analysis\n",
		"milestones.md":   "milestones\n",
		"epics.md":        "epics\n",
	}
	for name, content := range requiredPlanningDocs {
		path := filepath.Join(docsDir, name)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write planning doc %s: %v", name, err)
		}
	}
	adrDir := filepath.Join(docsDir, "adr")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir adr dir: %v", err)
	}
	adrPath := filepath.Join(adrDir, "adr-001.md")
	if err := os.WriteFile(adrPath, []byte("adr"), 0o644); err != nil {
		t.Fatalf("write adr file: %v", err)
	}
	tasksDir := filepath.Join(repoRoot, "_governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
}
