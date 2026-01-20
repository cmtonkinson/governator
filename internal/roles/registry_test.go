// Package roles provides tests for role prompt loading and selection.
package roles

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

// TestRegistryPromptFilesOrder ensures prompt file ordering matches the documented sequence.
func TestRegistryPromptFilesOrder(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "_governator", "roles", "engineer.md"), "role")
	writeFile(t, filepath.Join(root, "_governator", "custom-prompts", "_global.md"), "global")
	writeFile(t, filepath.Join(root, "_governator", "custom-prompts", "engineer.md"), "custom")

	registry, err := LoadRegistry(root, nil)
	if err != nil {
		t.Fatalf("LoadRegistry error: %v", err)
	}

	got := registry.PromptFiles(index.Role("engineer"))
	want := []string{
		"_governator/roles/engineer.md",
		"_governator/custom-prompts/_global.md",
		"_governator/custom-prompts/engineer.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PromptFiles = %#v, want %#v", got, want)
	}
}

// TestRegistryPromptFilesMissingRoleWarns ensures missing roles warn and still return optional prompts.
func TestRegistryPromptFilesMissingRoleWarns(t *testing.T) {
	t.Helper()
	root := t.TempDir()
	writeDir(t, filepath.Join(root, "_governator", "roles"))
	writeFile(t, filepath.Join(root, "_governator", "custom-prompts", "_global.md"), "global")
	writeFile(t, filepath.Join(root, "_governator", "custom-prompts", "ghost.md"), "custom")

	var warnings []string
	registry, err := LoadRegistry(root, func(message string) {
		warnings = append(warnings, message)
	})
	if err != nil {
		t.Fatalf("LoadRegistry error: %v", err)
	}

	got := registry.PromptFiles(index.Role("ghost"))
	want := []string{
		"_governator/custom-prompts/_global.md",
		"_governator/custom-prompts/ghost.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PromptFiles = %#v, want %#v", got, want)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %v, want 1 warning", warnings)
	}
	if warnings[0] != "missing role prompt for ghost" {
		t.Fatalf("warning = %q, want %q", warnings[0], "missing role prompt for ghost")
	}
}

// TestStageRoleSelectorUsesOverrides ensures stage overrides take precedence.
func TestStageRoleSelectorUsesOverrides(t *testing.T) {
	t.Helper()
	selector := StageRoleSelector{
		Default: index.Role("worker"),
		Overrides: map[Stage]index.Role{
			StageReview: "reviewer",
		},
	}
	role, err := selector.RoleForStage(StageReview)
	if err != nil {
		t.Fatalf("RoleForStage error: %v", err)
	}
	if role != "reviewer" {
		t.Fatalf("RoleForStage = %q, want %q", role, "reviewer")
	}
}

// TestStageRoleSelectorFallsBack ensures the default role is used when no override exists.
func TestStageRoleSelectorFallsBack(t *testing.T) {
	t.Helper()
	selector := StageRoleSelector{
		Default: "worker",
	}
	role, err := selector.RoleForStage(StageWork)
	if err != nil {
		t.Fatalf("RoleForStage error: %v", err)
	}
	if role != "worker" {
		t.Fatalf("RoleForStage = %q, want %q", role, "worker")
	}
}

// TestStageRoleSelectorErrorsWithoutDefault ensures missing defaults return an error.
func TestStageRoleSelectorErrorsWithoutDefault(t *testing.T) {
	t.Helper()
	selector := StageRoleSelector{}
	if _, err := selector.RoleForStage(StageWork); err == nil {
		t.Fatal("RoleForStage error = nil, want error")
	}
}

// writeDir ensures the directory exists.
func writeDir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
}

// writeFile writes the file after creating its parent directory.
func writeFile(t *testing.T, path string, contents string) {
	t.Helper()
	writeDir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("WriteFile error: %v", err)
	}
}
