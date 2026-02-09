package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/templates"
)

// TestApplyRepoMigrationsRejectsEmptyRepoRoot verifies early input validation.
func TestApplyRepoMigrationsRejectsEmptyRepoRoot(t *testing.T) {
	err := ApplyRepoMigrations("", InitOptions{})
	if err == nil {
		t.Fatal("expected error for empty repo root")
	}
	if !strings.Contains(err.Error(), "repo root cannot be empty") {
		t.Fatalf("expected repo root validation error, got: %v", err)
	}
}

// TestPendingRepoMigrationsRejectsEmptyRepoRoot verifies input validation.
func TestPendingRepoMigrationsRejectsEmptyRepoRoot(t *testing.T) {
	_, err := PendingRepoMigrations("")
	if err == nil {
		t.Fatal("expected error for empty repo root")
	}
	if !strings.Contains(err.Error(), "repo root cannot be empty") {
		t.Fatalf("expected repo root validation error, got: %v", err)
	}
}

// TestPendingRepoMigrationsIncludesMissingMarkers verifies pending IDs are returned when markers are absent.
func TestPendingRepoMigrationsIncludesMissingMarkers(t *testing.T) {
	repoRoot := t.TempDir()
	got, err := PendingRepoMigrations(repoRoot)
	if err != nil {
		t.Fatalf("PendingRepoMigrations: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least one pending migration")
	}
}

// TestPendingRepoMigrationsSkipsCompletedMarkers verifies completed migrations are excluded from pending output.
func TestPendingRepoMigrationsSkipsCompletedMarkers(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	got, err := PendingRepoMigrations(repoRoot)
	if err != nil {
		t.Fatalf("PendingRepoMigrations: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected no pending migrations, got: %v", got)
	}
}

// TestApplyRepoMigrationsCreatesConflictResolutionPrompt verifies the migration writes the embedded prompt and marker.
func TestApplyRepoMigrationsCreatesConflictResolutionPrompt(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	promptPath := filepath.Join(repoRoot, "_governator", "prompts", conflictResolutionPromptName)
	got, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read migrated prompt: %v", err)
	}
	want, err := templates.Read(conflictResolutionTemplatePath)
	if err != nil {
		t.Fatalf("read embedded template: %v", err)
	}
	if !bytes.Equal(bytes.TrimSpace(got), bytes.TrimSpace(want)) {
		t.Fatalf("migrated prompt does not match embedded template")
	}

	markerPath := filepath.Join(repoRoot, repoDurableStateDir, "migrations", conflictResolutionMigrationID+".done")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("migration marker missing: %v", err)
	}
	marker, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read migration marker: %v", err)
	}
	if string(marker) != "ok\n" {
		t.Fatalf("marker content = %q, want %q", string(marker), "ok\n")
	}
}

// TestApplyRepoMigrationsSkipsWhenTargetPromptExists ensures operator changes are preserved.
func TestApplyRepoMigrationsSkipsWhenTargetPromptExists(t *testing.T) {
	repoRoot := t.TempDir()
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	targetPath := filepath.Join(promptsDir, conflictResolutionPromptName)
	custom := []byte("custom prompt\n")
	if err := os.WriteFile(targetPath, custom, 0o644); err != nil {
		t.Fatalf("write existing prompt: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read existing prompt: %v", err)
	}
	if !bytes.Equal(got, custom) {
		t.Fatalf("existing prompt should not be overwritten")
	}
}

// TestApplyRepoMigrationsSkipsWhenSimilarPromptExists ensures migration avoids creating duplicates.
func TestApplyRepoMigrationsSkipsWhenSimilarPromptExists(t *testing.T) {
	repoRoot := t.TempDir()
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	similarPath := filepath.Join(promptsDir, "conflict_resolution.md")
	if err := os.WriteFile(similarPath, []byte("existing similar prompt\n"), 0o644); err != nil {
		t.Fatalf("write similar prompt: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	targetPath := filepath.Join(promptsDir, conflictResolutionPromptName)
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("target prompt should not be created when similar prompt exists")
	}
}

// TestApplyRepoMigrationsIsIdempotentPreservesPostMigrationEdits verifies reruns are marker-gated.
func TestApplyRepoMigrationsIsIdempotentPreservesPostMigrationEdits(t *testing.T) {
	repoRoot := t.TempDir()
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := os.MkdirAll(promptsDir, 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("first ApplyRepoMigrations: %v", err)
	}

	targetPath := filepath.Join(promptsDir, conflictResolutionPromptName)
	customAfterFirstRun := []byte("operator-edited-prompt\n")
	if err := os.WriteFile(targetPath, customAfterFirstRun, 0o644); err != nil {
		t.Fatalf("write operator prompt edit: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("second ApplyRepoMigrations: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("read prompt after second migration run: %v", err)
	}
	if !bytes.Equal(got, customAfterFirstRun) {
		t.Fatalf("idempotent rerun should preserve operator edits")
	}
}

// TestApplyRepoMigrationsSkipsWhenMarkerExists ensures completed migrations are not re-applied.
func TestApplyRepoMigrationsSkipsWhenMarkerExists(t *testing.T) {
	repoRoot := t.TempDir()
	migrationsDir := filepath.Join(repoRoot, repoDurableStateDir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	markerPath := filepath.Join(migrationsDir, conflictResolutionMigrationID+".done")
	if err := os.WriteFile(markerPath, []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	targetPath := filepath.Join(repoRoot, "_governator", "prompts", conflictResolutionPromptName)
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("prompt should not be created when marker exists")
	}
}

// TestApplyRepoMigrationsDoesNotWriteMarkerWhenMigrationFails verifies failure is explicit and non-committing.
func TestApplyRepoMigrationsDoesNotWriteMarkerWhenMigrationFails(t *testing.T) {
	repoRoot := t.TempDir()
	governatorRoot := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(governatorRoot, 0o755); err != nil {
		t.Fatalf("mkdir _governator: %v", err)
	}
	// Make prompts path invalid for ensureDir by creating a regular file.
	if err := os.WriteFile(filepath.Join(governatorRoot, "prompts"), []byte("not-a-directory"), 0o644); err != nil {
		t.Fatalf("write blocking prompts file: %v", err)
	}

	err := ApplyRepoMigrations(repoRoot, InitOptions{})
	if err == nil {
		t.Fatal("expected migration failure")
	}
	if !strings.Contains(err.Error(), "run migration "+conflictResolutionMigrationID) {
		t.Fatalf("expected wrapped migration id in error, got: %v", err)
	}

	markerPath := filepath.Join(repoRoot, repoDurableStateDir, "migrations", conflictResolutionMigrationID+".done")
	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("marker should not exist on migration failure")
	}
}

// TestSimilarPromptExists verifies exact, normalized, and keyword-based matching logic.
func TestSimilarPromptExists(t *testing.T) {
	testCases := []struct {
		name       string
		files      []string
		dirs       []string
		wantExists bool
	}{
		{
			name:       "exact name match",
			files:      []string{"conflict-resolution.md"},
			wantExists: true,
		},
		{
			name:       "case-insensitive exact match",
			files:      []string{"Conflict-Resolution.md"},
			wantExists: true,
		},
		{
			name:       "normalized stem match underscore",
			files:      []string{"conflict_resolution.md"},
			wantExists: true,
		},
		{
			name:       "normalized stem match punctuation",
			files:      []string{"Conflict Resolution!!.md"},
			wantExists: true,
		},
		{
			name:       "keyword heuristic conflict and resolution",
			files:      []string{"my-conflict-helper-resolution-prompt.txt"},
			wantExists: true,
		},
		{
			name:       "missing one keyword does not match",
			files:      []string{"conflict-helper.md"},
			wantExists: false,
		},
		{
			name:       "directory entries are ignored",
			dirs:       []string{"conflict_resolution.md"},
			wantExists: false,
		},
		{
			name:       "unrelated file does not match",
			files:      []string{"gap-analysis.md"},
			wantExists: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			promptsDir := t.TempDir()
			for _, name := range tc.files {
				if err := os.WriteFile(filepath.Join(promptsDir, name), []byte("x"), 0o644); err != nil {
					t.Fatalf("write file %s: %v", name, err)
				}
			}
			for _, name := range tc.dirs {
				if err := os.MkdirAll(filepath.Join(promptsDir, name), 0o755); err != nil {
					t.Fatalf("mkdir %s: %v", name, err)
				}
			}

			got := similarPromptExists(promptsDir, conflictResolutionPromptName)
			if got != tc.wantExists {
				t.Fatalf("similarPromptExists() = %v, want %v", got, tc.wantExists)
			}
		})
	}
}

// TestNormalizePromptStem verifies normalization behavior used by similarity matching.
func TestNormalizePromptStem(t *testing.T) {
	testCases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "hyphenated", input: "conflict-resolution.md", want: "conflictresolution"},
		{name: "underscored", input: "conflict_resolution.md", want: "conflictresolution"},
		{name: "spaces and punctuation", input: " Conflict Resolution!!.md ", want: "conflictresolution"},
		{name: "mixed extension case", input: "Conflict-Resolution.MD", want: "conflictresolution"},
		{name: "digits preserved", input: "conflict-resolution-v2.md", want: "conflictresolutionv2"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizePromptStem(tc.input)
			if got != tc.want {
				t.Fatalf("normalizePromptStem(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
