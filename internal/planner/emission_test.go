// Package planner tests planner emission behavior.
package planner

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

// TestEmitPlanHappyPath ensures planning emission writes plan artifacts, tasks, and index data.
func TestEmitPlanHappyPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "GOVERNATOR.md"), "governator content")
	writePowerSixDocs(t, root, true)

	output, err := ParsePlannerOutput([]byte(plannerOutputFixture))
	if err != nil {
		t.Fatalf("parse planner output: %v", err)
	}

	result, err := EmitPlan(root, output, PlanOptions{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("emit plan: %v", err)
	}

	planDir := filepath.Join(root, planDirName)
	expectPlanFile(t, filepath.Join(planDir, architecturePlanFile))
	expectPlanFile(t, filepath.Join(planDir, roadmapPlanFile))
	expectPlanFile(t, filepath.Join(planDir, taskGenerationPlanFile))
	if _, err := os.Stat(filepath.Join(planDir, gapAnalysisPlanFile)); err == nil {
		t.Fatalf("expected %s to be absent", gapAnalysisPlanFile)
	}

	task := output.Tasks.Tasks[0]
	taskPath := filepath.Join(root, tasksDirName, taskFileName(task.ID, task.Title))
	content := readFile(t, taskPath)
	if !strings.Contains(content, "# Task: "+task.Title) {
		t.Fatalf("expected task file to include title %q", task.Title)
	}
	if !strings.Contains(content, "task: "+task.ID) {
		t.Fatalf("expected task file to include id %q", task.ID)
	}

	indexPath := filepath.Join(root, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	if idx.SchemaVersion != indexSchemaVersion {
		t.Fatalf("schema_version = %d, want %d", idx.SchemaVersion, indexSchemaVersion)
	}
	if idx.Digests.GovernatorMD == "" {
		t.Fatal("expected governator_md digest to be set")
	}
	if len(idx.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(idx.Tasks))
	}
	expectedPath := filepath.ToSlash(filepath.Join(tasksDirName, taskFileName(task.ID, task.Title)))
	if idx.Tasks[0].ID != task.ID || idx.Tasks[0].Title != task.Title || idx.Tasks[0].Path != expectedPath {
		t.Fatalf("index task mismatch: got id=%q title=%q path=%q", idx.Tasks[0].ID, idx.Tasks[0].Title, idx.Tasks[0].Path)
	}
	expectedOverlap := []string{"index", "planner"}
	if !reflect.DeepEqual(idx.Tasks[0].Overlap, expectedOverlap) {
		t.Fatalf("overlap = %v, want %v", idx.Tasks[0].Overlap, expectedOverlap)
	}
	if result.IndexPath != filepath.ToSlash(indexFilePath) {
		t.Fatalf("result index path = %q, want %q", result.IndexPath, filepath.ToSlash(indexFilePath))
	}
	if len(result.PlanFiles) != 3 {
		t.Fatalf("expected 3 plan files, got %d", len(result.PlanFiles))
	}
}

// TestEmitPlanMissingPowerSix ensures required bootstrap artifacts are enforced.
func TestEmitPlanMissingPowerSix(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "GOVERNATOR.md"), "governator content")

	output, err := ParsePlannerOutput([]byte(plannerOutputFixture))
	if err != nil {
		t.Fatalf("parse planner output: %v", err)
	}

	_, err = EmitPlan(root, output, PlanOptions{})
	if err == nil {
		t.Fatal("expected error for missing Power Six docs")
	}
	if !strings.Contains(err.Error(), "missing required Power Six doc") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// expectPlanFile asserts a planning artifact is present.
func expectPlanFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected plan artifact %s: %v", path, err)
	}
}

// readFile reads a file into a string for assertions.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
