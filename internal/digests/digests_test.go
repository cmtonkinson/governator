// Package digests tests digest computation and drift detection.
package digests

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeDigests(t *testing.T) {
	root := t.TempDir()
	docsDir := filepath.Join(root, "_governator", "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir plan dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, ".keep"), []byte(""), 0o644); err != nil {
		t.Fatalf("write keep: %v", err)
	}
	governatorContent := "governator\n"
	if err := os.WriteFile(filepath.Join(root, "GOVERNATOR.md"), []byte(governatorContent), 0o644); err != nil {
		t.Fatalf("write GOV: %v", err)
	}
	planPath := filepath.Join(docsDir, "roadmap.md")
	planContent := "plan\n"
	if err := os.WriteFile(planPath, []byte(planContent), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}

	got, err := Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	if got.GovernatorMD != "sha256:328961dd5885fa93c7c1f184d3489723f202e870088c9ae747f1454dc406176a" {
		t.Fatalf("unexpected governator digest: %s", got.GovernatorMD)
	}
	relativePlan := filepath.ToSlash(filepath.Join("_governator", "docs", "roadmap.md"))
	if got.PlanningDocs[relativePlan] != digestForString(planContent) {
		t.Fatalf("unexpected plan digest: %s", got.PlanningDocs[relativePlan])
	}
	for path := range got.PlanningDocs {
		if filepath.Base(path) == ".keep" {
			t.Fatalf("unexpected .keep digest for %s", path)
		}
	}
}

func TestDetectDriftNoChanges(t *testing.T) {
	root := t.TempDir()
	if err := writeRepoFixture(root); err != nil {
		t.Fatalf("write repo: %v", err)
	}

	stored, err := Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	report, err := Detect(root, stored)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if report.HasDrift {
		t.Fatalf("expected no drift, got %v", report.Details)
	}
}

func TestDetectDriftGoverningDocChange(t *testing.T) {
	root := t.TempDir()
	if err := writeRepoFixture(root); err != nil {
		t.Fatalf("write repo: %v", err)
	}

	stored, err := Compute(root)
	if err != nil {
		t.Fatalf("Compute error: %v", err)
	}

	if err := os.WriteFile(filepath.Join(root, "GOVERNATOR.md"), []byte("changed\n"), 0o644); err != nil {
		t.Fatalf("update GOV: %v", err)
	}

	report, err := Detect(root, stored)
	if err != nil {
		t.Fatalf("Detect error: %v", err)
	}
	if !report.HasDrift {
		t.Fatal("expected drift")
	}
	if !strings.Contains(report.Message, "GOVERNATOR.md changed") {
		t.Fatalf("expected drift message, got %q", report.Message)
	}
	if !strings.Contains(report.Message, "replan required") {
		t.Fatalf("expected replanning prompt, got %q", report.Message)
	}
}

func writeRepoFixture(root string) error {
	docsDir := filepath.Join(root, "_governator", "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return fmt.Errorf("mkdir docs dir: %w", err)
	}
	if err := os.WriteFile(filepath.Join(root, "GOVERNATOR.md"), []byte("governator\n"), 0o644); err != nil {
		return fmt.Errorf("write GOV: %w", err)
	}
	if err := os.WriteFile(filepath.Join(docsDir, "roadmap.md"), []byte("plan\n"), 0o644); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}

func digestForString(content string) string {
	sum := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", sum)
}

func TestCompute_IgnoresPlanningNotes(t *testing.T) {
	t.Parallel()
	repoRoot := t.TempDir()

	// Create docs dir with planning-notes.md
	docsDir := filepath.Join(repoRoot, "_governator", "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	planningNotes := filepath.Join(docsDir, "planning-notes.md")
	if err := os.WriteFile(planningNotes, []byte("test content"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Also create GOVERNATOR.md to make Compute happy
	if err := os.WriteFile(filepath.Join(repoRoot, "GOVERNATOR.md"), []byte("test"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Compute digests
	digests, err := Compute(repoRoot)
	if err != nil {
		t.Fatalf("Compute() failed: %v", err)
	}

	// planning-notes.md should NOT be in the digests map
	for path := range digests.PlanningDocs {
		if strings.Contains(path, "planning-notes.md") {
			t.Errorf("planning-notes.md should be excluded from digests, but found: %s", path)
		}
	}
}
