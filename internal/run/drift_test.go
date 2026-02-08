// Package run tests drift checks used by the run command.
package run

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/digests"
)

// TestCheckPlanningDriftNoChanges ensures clean repos pass the drift check.
func TestCheckPlanningDriftNoChanges(t *testing.T) {
	root := t.TempDir()
	if err := writeRepoFixture(root); err != nil {
		t.Fatalf("write repo: %v", err)
	}

	stored, err := digests.Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	if err := CheckPlanningDrift(root, stored); err != nil {
		t.Fatalf("unexpected drift error: %v", err)
	}
}

// TestCheckPlanningDriftDetectsPlanningDocChange ensures changed planning docs trigger replanning.
func TestCheckPlanningDriftDetectsPlanningDocChange(t *testing.T) {
	root := t.TempDir()
	if err := writeRepoFixture(root); err != nil {
		t.Fatalf("write repo: %v", err)
	}

	stored, err := digests.Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	roadmapPath := filepath.Join(root, "_governator", "docs", "roadmap.md")
	if err := os.WriteFile(roadmapPath, []byte("changed plan\n"), 0o644); err != nil {
		t.Fatalf("update roadmap: %v", err)
	}

	err = CheckPlanningDrift(root, stored)
	assertPlanningDriftError(t, err)
	assertErrorContains(t, err, "planning doc changed: _governator/docs/roadmap.md")
}

// TestCheckPlanningDriftDetectsPlanningDocDeletion ensures deleted planning docs trigger replanning.
func TestCheckPlanningDriftDetectsPlanningDocDeletion(t *testing.T) {
	root := t.TempDir()
	if err := writeRepoFixture(root); err != nil {
		t.Fatalf("write repo: %v", err)
	}

	stored, err := digests.Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	roadmapPath := filepath.Join(root, "_governator", "docs", "roadmap.md")
	if err := os.Remove(roadmapPath); err != nil {
		t.Fatalf("remove roadmap: %v", err)
	}

	err = CheckPlanningDrift(root, stored)
	assertPlanningDriftError(t, err)
	assertErrorContains(t, err, "planning doc missing: _governator/docs/roadmap.md")
}

// TestCheckPlanningDriftDetectsNonADRDocAddition ensures non-ADR additions trigger replanning.
func TestCheckPlanningDriftDetectsNonADRDocAddition(t *testing.T) {
	root := t.TempDir()
	if err := writeRepoFixture(root); err != nil {
		t.Fatalf("write repo: %v", err)
	}

	stored, err := digests.Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "_governator", "docs", "implementation-plan.md"), []byte("# plan\n"), 0o644); err != nil {
		t.Fatalf("write planning doc: %v", err)
	}

	err = CheckPlanningDrift(root, stored)
	assertPlanningDriftError(t, err)
	assertErrorContains(t, err, "planning doc added: _governator/docs/implementation-plan.md")
}

func assertPlanningDriftError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected drift error")
	}
	if !errors.Is(err, ErrPlanningDrift) {
		t.Fatalf("expected ErrPlanningDrift, got %v", err)
	}
	assertErrorContains(t, err, "Planning drift detected; replan required.")
	assertErrorContains(t, err, "governator start")
}

func assertErrorContains(t *testing.T, err error, expected string) {
	t.Helper()
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected error to contain %q, got %q", expected, err.Error())
	}
}

// writeRepoFixture creates minimal planning artifacts for drift checks.
func writeRepoFixture(root string) error {
	docsDir := filepath.Join(root, "_governator", "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(root, "GOVERNATOR.md"), []byte("governator\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(docsDir, "roadmap.md"), []byte("plan\n"), 0o644); err != nil {
		return err
	}
	return nil
}
