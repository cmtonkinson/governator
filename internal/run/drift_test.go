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

// TestCheckPlanningDriftDetectsChange ensures drift stops the run with guidance.
func TestCheckPlanningDriftDetectsChange(t *testing.T) {
	root := t.TempDir()
	if err := writeRepoFixture(root); err != nil {
		t.Fatalf("write repo: %v", err)
	}

	stored, err := digests.Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	adrDir := filepath.Join(root, "_governator", "docs", "adr")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("create ADR dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(adrDir, "adr-0002-test.md"), []byte("# ADR\n"), 0o644); err != nil {
		t.Fatalf("write ADR: %v", err)
	}

	err = CheckPlanningDrift(root, stored)
	if err == nil {
		t.Fatal("expected drift error")
	}
	if !errors.Is(err, ErrPlanningDrift) {
		t.Fatalf("expected ErrPlanningDrift, got %v", err)
	}
	message := err.Error()
	if !strings.Contains(message, "ADR drift detected") {
		t.Fatalf("expected drift message, got %q", message)
	}
	if !strings.Contains(message, "ADR added") {
		t.Fatalf("expected drift details, got %q", message)
	}
	if !strings.Contains(message, "governator start") {
		t.Fatalf("expected replanning guidance, got %q", message)
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
