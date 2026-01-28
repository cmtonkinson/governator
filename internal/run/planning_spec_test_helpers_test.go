package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/templates"
)

// writeTestPlanningSpec writes the embedded planning spec to the repo root for tests.
func writeTestPlanningSpec(t *testing.T, repoRoot string) {
	t.Helper()
	data, err := templates.Read("planning/planning.json")
	if err != nil {
		t.Fatalf("read planning spec template: %v", err)
	}
	path := filepath.Join(repoRoot, "_governator", "planning.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir planning spec dir: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write planning spec: %v", err)
	}
}
