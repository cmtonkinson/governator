package phase

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/bootstrap"
)

func TestStoreStatePersistence(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	store := NewStore(repoRoot)

	state, err := store.Load()
	if err != nil {
		t.Fatalf("load default state: %v", err)
	}
	if state.Current != PhaseArchitectureBaseline {
		t.Fatalf("expected current phase architecture baseline, got %v", state.Current)
	}

	state.Current = PhaseTaskPlanning
	if err := store.Save(state); err != nil {
		t.Fatalf("save state: %v", err)
	}

	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load saved state: %v", err)
	}
	if loaded.Current != state.Current {
		t.Fatalf("current phase mismatch: got %v want %v", loaded.Current, state.Current)
	}
}

func TestValidateArchitectureArtifacts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	docs := filepath.Join(repoRoot, docsDirName)
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("create docs dir: %v", err)
	}

	for _, artifact := range bootstrap.Artifacts() {
		if !artifact.Required {
			continue
		}
		path := filepath.Join(docs, artifact.Name)
		if err := os.WriteFile(path, []byte("content"), 0o644); err != nil {
			t.Fatalf("write artifact %s: %v", artifact.Name, err)
		}
	}

	validations, err := ValidatePrerequisites(repoRoot, PhaseGapAnalysis)
	if err != nil {
		t.Fatalf("validate prerequisites: %v", err)
	}
	for _, validation := range validations {
		if !validation.Valid {
			t.Fatalf("expected %s to be valid", validation.Name)
		}
	}
}

func TestValidateGapReport(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	docs := filepath.Join(repoRoot, docsDirName)
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("create docs dir: %v", err)
	}

	reportPath := filepath.Join(docs, gapFileName)
	if err := os.WriteFile(reportPath, []byte("gap"), 0o644); err != nil {
		t.Fatalf("write gap report: %v", err)
	}

	validations, err := ValidatePrerequisites(repoRoot, PhaseProjectPlanning)
	if err != nil {
		t.Fatalf("validate prerequisites: %v", err)
	}
	if len(validations) != 1 {
		t.Fatalf("expected single validation entry, got %d", len(validations))
	}
	if !validations[0].Valid {
		t.Fatalf("expected gap report validation to succeed")
	}
}

func TestValidateRoadmapArtifacts(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	docs := filepath.Join(repoRoot, docsDirName)
	if err := os.MkdirAll(docs, 0o755); err != nil {
		t.Fatalf("create docs dir: %v", err)
	}

	for _, name := range []string{milestones, epics} {
		if err := os.WriteFile(filepath.Join(docs, name), []byte("roadmap"), 0o644); err != nil {
			t.Fatalf("write roadmap artifact %s: %v", name, err)
		}
	}

	validations, err := ValidatePrerequisites(repoRoot, PhaseTaskPlanning)
	if err != nil {
		t.Fatalf("validate prerequisites: %v", err)
	}
	for _, validation := range validations {
		if !validation.Valid {
			t.Fatalf("expected roadmap artifact %s to be valid", validation.Name)
		}
	}
}

func TestValidateTaskBacklog(t *testing.T) {
	t.Parallel()

	repoRoot := t.TempDir()
	tasks := filepath.Join(repoRoot, tasksDirName)
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasks, "000-task.md"), []byte("task"), 0o644); err != nil {
		t.Fatalf("write task file: %v", err)
	}

	validations, err := ValidatePrerequisites(repoRoot, PhaseExecution)
	if err != nil {
		t.Fatalf("validate prerequisites: %v", err)
	}
	if len(validations) != 1 {
		t.Fatalf("expected single validation entry, got %d", len(validations))
	}
	if !validations[0].Valid {
		t.Fatalf("expected task backlog validation to succeed")
	}
}
