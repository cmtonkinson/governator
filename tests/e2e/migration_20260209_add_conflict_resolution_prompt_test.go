// Package test provides end-to-end coverage for repository migration behavior.
package test

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/run"
	"github.com/cmtonkinson/governator/internal/templates"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

const (
	e2eConflictPromptName      = "conflict-resolution.md"
	e2eConflictTemplatePath    = "planning/conflict-resolution.md"
	e2eConflictMigrationID     = "20260209_add_conflict_resolution_prompt"
	e2eConflictMigrationMarker = "_governator/_durable-state/migrations/20260209_add_conflict_resolution_prompt.done"
)

// TestE2EMigrationsCreatesConflictPrompt verifies startup applies migrations and writes the embedded prompt.
func TestE2EMigrationsCreatesConflictPrompt(t *testing.T) {
	repoRoot := setupMigrationReadyRepo(t)
	promptPath := filepath.Join(repoRoot, "_governator", "prompts", e2eConflictPromptName)
	if err := os.Remove(promptPath); err != nil {
		t.Fatalf("remove prompt before migration test: %v", err)
	}

	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("run unified supervisor: %v", err)
	}

	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read migrated prompt: %v", err)
	}
	want, err := templates.Read(e2eConflictTemplatePath)
	if err != nil {
		t.Fatalf("read embedded template: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("migrated prompt does not match embedded template")
	}

	assertMigrationMarker(t, repoRoot, true)
}

// TestE2EMigrationsSkipsWhenTargetExists verifies migration does not overwrite an existing prompt.
func TestE2EMigrationsSkipsWhenTargetExists(t *testing.T) {
	repoRoot := setupMigrationReadyRepo(t)
	promptPath := filepath.Join(repoRoot, "_governator", "prompts", e2eConflictPromptName)
	custom := []byte("operator-customized conflict prompt\n")
	if err := os.WriteFile(promptPath, custom, 0o644); err != nil {
		t.Fatalf("seed custom conflict prompt: %v", err)
	}

	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("run unified supervisor: %v", err)
	}

	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	if !bytes.Equal(got, custom) {
		t.Fatalf("existing prompt was overwritten by migration")
	}

	assertMigrationMarker(t, repoRoot, true)
}

// TestE2EMigrationsSkipsWhenSimilarPromptExists verifies startup migration avoids duplicate prompt creation.
func TestE2EMigrationsSkipsWhenSimilarPromptExists(t *testing.T) {
	repoRoot := setupMigrationReadyRepo(t)
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	targetPath := filepath.Join(promptsDir, e2eConflictPromptName)
	if err := os.Remove(targetPath); err != nil {
		t.Fatalf("remove canonical conflict prompt: %v", err)
	}
	similarPath := filepath.Join(promptsDir, "conflict_resolution.md")
	if err := os.WriteFile(similarPath, []byte("existing similar prompt\n"), 0o644); err != nil {
		t.Fatalf("write similar prompt: %v", err)
	}

	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("run unified supervisor: %v", err)
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("canonical prompt should not be created when similar prompt exists")
	}
	assertMigrationMarker(t, repoRoot, true)
}

// TestE2EMigrationsSkipsWhenMarkerExists verifies migration short-circuits when already marked complete.
func TestE2EMigrationsSkipsWhenMarkerExists(t *testing.T) {
	repoRoot := setupMigrationReadyRepo(t)
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	targetPath := filepath.Join(promptsDir, e2eConflictPromptName)
	if err := os.Remove(targetPath); err != nil {
		t.Fatalf("remove canonical conflict prompt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, e2eConflictMigrationMarker), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("seed migration marker: %v", err)
	}

	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("run unified supervisor: %v", err)
	}

	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("canonical prompt should not be created when marker exists")
	}
	assertMigrationMarker(t, repoRoot, true)
}

// TestE2EMigrationsFailsWhenPromptsPathInvalid verifies startup fails fast when migration cannot create prompts directory.
func TestE2EMigrationsFailsWhenPromptsPathInvalid(t *testing.T) {
	repoRoot := setupMigrationReadyRepo(t)
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := os.RemoveAll(promptsDir); err != nil {
		t.Fatalf("remove prompts directory: %v", err)
	}
	if err := os.WriteFile(promptsDir, []byte("not-a-directory"), 0o644); err != nil {
		t.Fatalf("write blocking prompts file: %v", err)
	}

	err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected unified supervisor startup to fail")
	}
	if !strings.Contains(err.Error(), "apply repo migrations") {
		t.Fatalf("expected migration startup failure, got: %v", err)
	}
	if !strings.Contains(err.Error(), e2eConflictMigrationID) {
		t.Fatalf("expected migration id in error, got: %v", err)
	}

	assertMigrationMarker(t, repoRoot, false)
}

// TestE2EMigrationsRerunPreservesPrompt verifies idempotent reruns preserve operator edits.
func TestE2EMigrationsRerunPreservesPrompt(t *testing.T) {
	repoRoot := setupMigrationReadyRepo(t)
	promptPath := filepath.Join(repoRoot, "_governator", "prompts", e2eConflictPromptName)
	if err := os.Remove(promptPath); err != nil {
		t.Fatalf("remove prompt before migration test: %v", err)
	}

	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("first unified supervisor run: %v", err)
	}

	operatorContent := []byte("operator-edited after first migration\n")
	if err := os.WriteFile(promptPath, operatorContent, 0o644); err != nil {
		t.Fatalf("write operator content: %v", err)
	}

	if err := run.RunUnifiedSupervisor(repoRoot, run.UnifiedSupervisorOptions{
		Stdout:       io.Discard,
		Stderr:       io.Discard,
		PollInterval: 10 * time.Millisecond,
	}); err != nil {
		t.Fatalf("second unified supervisor run: %v", err)
	}

	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt after rerun: %v", err)
	}
	if !bytes.Equal(got, operatorContent) {
		t.Fatalf("operator prompt content should be preserved across reruns")
	}
	assertMigrationMarker(t, repoRoot, true)
}

// setupMigrationReadyRepo creates a repository with complete startup prerequisites and no migration marker.
func setupMigrationReadyRepo(t *testing.T) string {
	t.Helper()

	repo := testrepos.New(t)
	repoRoot := repo.Root
	TrackE2ERepo(t, repoRoot)

	governatorPath := filepath.Join(repoRoot, "GOVERNATOR.md")
	if err := os.WriteFile(governatorPath, []byte("Migration e2e test repository.\n"), 0o644); err != nil {
		t.Fatalf("write GOVERNATOR.md: %v", err)
	}

	if err := config.InitFullLayout(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("init full layout: %v", err)
	}
	if err := run.SeedPlanningIndex(repoRoot); err != nil {
		t.Fatalf("seed planning index: %v", err)
	}

	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
	idx, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}
	for i := range idx.Tasks {
		if idx.Tasks[i].ID == "planning" && idx.Tasks[i].Kind == index.TaskKindPlanning {
			idx.Tasks[i].State = index.TaskState(run.PlanningCompleteState)
		}
	}
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	markerPath := filepath.Join(repoRoot, e2eConflictMigrationMarker)
	if err := os.Remove(markerPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove migration marker: %v", err)
	}

	repo.RunGit(t, "add", ".")
	repo.RunGit(t, "commit", "-m", "Seed migration e2e fixture")

	return repoRoot
}

// assertMigrationMarker validates marker presence/absence and expected content.
func assertMigrationMarker(t *testing.T, repoRoot string, wantExists bool) {
	t.Helper()
	markerPath := filepath.Join(repoRoot, e2eConflictMigrationMarker)
	data, err := os.ReadFile(markerPath)
	if wantExists {
		if err != nil {
			t.Fatalf("read migration marker: %v", err)
		}
		if string(data) != "ok\n" {
			t.Fatalf("migration marker content = %q, want %q", string(data), "ok\n")
		}
		return
	}
	if !os.IsNotExist(err) {
		t.Fatalf("migration marker should be absent, got err=%v", err)
	}
}
