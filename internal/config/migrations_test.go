package config

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
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
	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", "prompts"), 0o755); err != nil {
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
	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	promptPath := filepath.Join(repoRoot, ".governator", "prompts", conflictResolutionPromptName)
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
	promptsDir := filepath.Join(repoRoot, ".governator", "prompts")
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
	promptsDir := filepath.Join(repoRoot, ".governator", "prompts")
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
	promptsDir := filepath.Join(repoRoot, ".governator", "prompts")
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

	targetPath := filepath.Join(repoRoot, ".governator", "prompts", conflictResolutionPromptName)
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("prompt should not be created when marker exists")
	}
}

// TestApplyRepoMigrationsDoesNotWriteMarkerWhenMigrationFails verifies failure is explicit and non-committing.
func TestApplyRepoMigrationsDoesNotWriteMarkerWhenMigrationFails(t *testing.T) {
	repoRoot := t.TempDir()
	governatorRoot := filepath.Join(repoRoot, ".governator")
	if err := os.MkdirAll(governatorRoot, 0o755); err != nil {
		t.Fatalf("mkdir .governator: %v", err)
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

// TestPendingRepoMigrationInfoIncludesDestructiveMigration verifies destructive metadata is exposed for confirmation gates.
func TestPendingRepoMigrationInfoIncludesDestructiveMigration(t *testing.T) {
	repoRoot := t.TempDir()
	info, err := PendingRepoMigrationInfo(repoRoot)
	if err != nil {
		t.Fatalf("PendingRepoMigrationInfo: %v", err)
	}

	found := false
	for _, migration := range info {
		if migration.ID == resetOpenTasksMigrationID {
			found = true
			if !migration.Destructive {
				t.Fatalf("migration %s should be destructive", resetOpenTasksMigrationID)
			}
		}
	}
	if !found {
		t.Fatalf("expected migration %s in pending list", resetOpenTasksMigrationID)
	}
}

// TestApplyRepoMigrationsResetOpenTasksAndStripRoleSuffix verifies task reset, file rename, branch/worktree cleanup, and in-flight reset.
func TestApplyRepoMigrationsResetOpenTasksAndStripRoleSuffix(t *testing.T) {
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)
	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", "roles"), 0o755); err != nil {
		t.Fatalf("mkdir roles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".governator", "roles", "architect.md"), []byte("# Architect\n"), 0o644); err != nil {
		t.Fatalf("write architect role: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".governator", "roles", "default.md"), []byte("# Default\n"), 0o644); err != nil {
		t.Fatalf("write default role: %v", err)
	}

	tasksDir := filepath.Join(repoRoot, ".governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	for _, name := range []string{
		"001-build-api-architect.md",
		"002-test-api-default.md",
		"003-release-notes.md",
	} {
		if err := os.WriteFile(filepath.Join(tasksDir, name), []byte("# Task\n"), 0o644); err != nil {
			t.Fatalf("write task %s: %v", name, err)
		}
	}

	idx := index.Index{
		SchemaVersion: 1,
		Digests:       index.Digests{PlanningDocs: map[string]string{}},
		Tasks: []index.Task{
			{
				ID:    "planning",
				Path:  ".governator/planning.json",
				Kind:  index.TaskKindPlanning,
				State: index.TaskState("governator_planning_complete"),
				Role:  "planner",
			},
			{
				ID:           "001-build-api-architect",
				Path:         ".governator/tasks/001-build-api-architect.md",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateTriaged,
				Role:         "architect",
				Dependencies: []string{},
			},
			{
				ID:            "002-test-api-default",
				Path:          ".governator/tasks/002-test-api-default.md",
				Kind:          index.TaskKindExecution,
				State:         index.TaskStateBlocked,
				Role:          "default",
				Dependencies:  []string{"001-build-api-architect"},
				Attempts:      index.AttemptCounters{Total: 2, Failed: 2},
				BlockedReason: "timed out",
			},
			{
				ID:           "003-release-notes",
				Path:         ".governator/tasks/003-release-notes.md",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateMerged,
				Role:         "default",
				Dependencies: []string{"002-test-api-default"},
			},
		},
	}
	indexPath := filepath.Join(repoRoot, ".governator", ".local-state", "index.json")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatalf("mkdir index dir: %v", err)
	}
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	store, err := inflight.NewStore(repoRoot)
	if err != nil {
		t.Fatalf("new in-flight store: %v", err)
	}
	if err := store.Save(inflight.Set{
		"001-build-api-architect": {ID: "001-build-api-architect"},
		"002-test-api-default":    {ID: "002-test-api-default"},
	}); err != nil {
		t.Fatalf("seed in-flight: %v", err)
	}

	runGitCommand(t, repoRoot, "branch", "001-build-api-architect")
	runGitCommand(t, repoRoot, "branch", "task-002-test-api-default")
	worktreePath := filepath.Join(repoRoot, ".governator", "worktrees", "task-001-build-api-architect")
	runGitCommand(t, repoRoot, "worktree", "add", worktreePath, "001-build-api-architect")
	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", ".local-state", "meta"), 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".governator", ".local-state", "meta", "001-build-api-architect.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write meta: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	updated, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load updated index: %v", err)
	}
	taskByID := map[string]index.Task{}
	for _, task := range updated.Tasks {
		taskByID[task.ID] = task
	}
	planningTask, ok := taskByID["planning"]
	if !ok {
		t.Fatalf("planning task missing")
	}
	if planningTask.State != index.TaskState("governator_planning_complete") {
		t.Fatalf("planning state changed: got %s", planningTask.State)
	}
	taskA, ok := taskByID["001-build-api"]
	if !ok {
		t.Fatalf("renamed task 001-build-api missing")
	}
	if taskA.State != index.TaskStateBacklog {
		t.Fatalf("taskA state = %s, want backlog", taskA.State)
	}
	if taskA.Path != ".governator/tasks/001-build-api.md" {
		t.Fatalf("taskA path = %s, want .governator/tasks/001-build-api.md", taskA.Path)
	}

	taskB, ok := taskByID["002-test-api"]
	if !ok {
		t.Fatalf("renamed task 002-test-api missing")
	}
	if taskB.State != index.TaskStateBacklog {
		t.Fatalf("taskB state = %s, want backlog", taskB.State)
	}
	if taskB.Role != "" {
		t.Fatalf("taskB role = %q, want empty", taskB.Role)
	}
	if len(taskB.Dependencies) != 1 || taskB.Dependencies[0] != "001-build-api" {
		t.Fatalf("taskB dependencies = %#v, want [001-build-api]", taskB.Dependencies)
	}
	if taskB.Attempts.Total != 0 || taskB.Attempts.Failed != 0 {
		t.Fatalf("taskB attempts should be reset, got %#v", taskB.Attempts)
	}

	taskC, ok := taskByID["003-release-notes"]
	if !ok {
		t.Fatalf("task 003-release-notes missing")
	}
	if len(taskC.Dependencies) != 1 || taskC.Dependencies[0] != "002-test-api" {
		t.Fatalf("taskC dependencies = %#v, want [002-test-api]", taskC.Dependencies)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "tasks", "001-build-api.md")); err != nil {
		t.Fatalf("renamed file 001-build-api.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "tasks", "001-build-api-architect.md")); !os.IsNotExist(err) {
		t.Fatalf("old suffixed file should be removed, stat err=%v", err)
	}

	clearedInFlight, err := store.Load()
	if err != nil {
		t.Fatalf("load in-flight: %v", err)
	}
	if len(clearedInFlight) != 0 {
		t.Fatalf("expected in-flight cleared, got %#v", clearedInFlight)
	}

	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree path should be removed, stat err=%v", err)
	}
	if branchExists(t, repoRoot, "001-build-api-architect") {
		t.Fatalf("expected branch 001-build-api-architect to be deleted")
	}
	if branchExists(t, repoRoot, "task-002-test-api-default") {
		t.Fatalf("expected legacy branch task-002-test-api-default to be deleted")
	}

	markerPath := filepath.Join(repoRoot, repoDurableStateDir, "migrations", resetOpenTasksMigrationID+".done")
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("new migration marker missing: %v", err)
	}
}

// TestApplyRepoMigrationsResetOpenTasksStripRoleSuffixFailsOnCollision verifies rename collisions fail and do not mark completion.
func TestApplyRepoMigrationsResetOpenTasksStripRoleSuffixFailsOnCollision(t *testing.T) {
	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", "roles"), 0o755); err != nil {
		t.Fatalf("mkdir roles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".governator", "roles", "architect.md"), []byte("# Architect\n"), 0o644); err != nil {
		t.Fatalf("write architect role: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, ".governator", "roles", "planner.md"), []byte("# Planner\n"), 0o644); err != nil {
		t.Fatalf("write planner role: %v", err)
	}

	tasksDir := filepath.Join(repoRoot, ".governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		t.Fatalf("mkdir tasks: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "001-feature-architect.md"), []byte("# one\n"), 0o644); err != nil {
		t.Fatalf("write task one: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "001-feature-planner.md"), []byte("# two\n"), 0o644); err != nil {
		t.Fatalf("write task two: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Digests:       index.Digests{PlanningDocs: map[string]string{}},
		Tasks: []index.Task{
			{ID: "001-feature-architect", Path: ".governator/tasks/001-feature-architect.md", Kind: index.TaskKindExecution, State: index.TaskStateTriaged},
			{ID: "001-feature-planner", Path: ".governator/tasks/001-feature-planner.md", Kind: index.TaskKindExecution, State: index.TaskStateTriaged},
		},
	}
	indexPath := filepath.Join(repoRoot, ".governator", ".local-state", "index.json")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatalf("mkdir index dir: %v", err)
	}
	raw, err := json.Marshal(idx)
	if err != nil {
		t.Fatalf("marshal index: %v", err)
	}
	if err := os.WriteFile(indexPath, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	err = ApplyRepoMigrations(repoRoot, InitOptions{})
	if err == nil {
		t.Fatal("expected migration failure due to rename collision")
	}
	if !strings.Contains(err.Error(), "rename collision") {
		t.Fatalf("expected rename collision error, got: %v", err)
	}

	markerPath := filepath.Join(repoRoot, repoDurableStateDir, "migrations", resetOpenTasksMigrationID+".done")
	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("destructive migration marker should be absent on failure")
	}
}

// TestApplyRepoMigrationsMigratesLegacyWorkspaceLayout verifies legacy _governator paths are migrated to the dot-prefixed layout.
func TestApplyRepoMigrationsMigratesLegacyWorkspaceLayout(t *testing.T) {
	repoRoot := t.TempDir()

	legacyRoot := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(filepath.Join(legacyRoot, "_durable-state", "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir legacy durable state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "_durable-state", "config.json"), []byte("{\"branches\":{\"base\":\"main\"}}\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "_durable-state", "migrations", conflictResolutionMigrationID+".done"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write legacy conflict marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "_durable-state", "migrations", resetOpenTasksMigrationID+".done"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write legacy reset marker: %v", err)
	}

	legacyLocalState := filepath.Join(legacyRoot, "_local-state")
	if err := os.MkdirAll(filepath.Join(legacyLocalState, "meta"), 0o755); err != nil {
		t.Fatalf("mkdir legacy meta dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyLocalState, "index.json"), []byte("{\"schema_version\":1,\"tasks\":[],\"digests\":{\"planning_docs\":{}}}\n"), 0o644); err != nil {
		t.Fatalf("write legacy index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyLocalState, "dag.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write legacy dag: %v", err)
	}
	legacyTaskWorktree := filepath.Join(legacyLocalState, "task-T-001")
	if err := os.MkdirAll(legacyTaskWorktree, 0o755); err != nil {
		t.Fatalf("mkdir legacy task worktree: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(legacyLocalState, "merge-worktrees", "tmp"), 0o755); err != nil {
		t.Fatalf("mkdir legacy merge worktrees: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyLocalState, "merge-worktrees", "tmp", "touch.txt"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write legacy merge worktree file: %v", err)
	}
	metaPath := filepath.Join(legacyLocalState, "meta", "T-001.json")
	metaContent := []byte("{\"worktree_rel_path\":\"_governator/_local-state/task-T-001\",\"branch\":\"task-T-001\"}\n")
	if err := os.WriteFile(metaPath, metaContent, 0o644); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}

	if err := os.WriteFile(filepath.Join(legacyRoot, ".gitignore"), []byte("_local-state/*\n!_local-state/.keep\n"), 0o644); err != nil {
		t.Fatalf("write legacy gitignore: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "state", "config.json")); err != nil {
		t.Fatalf("expected migrated config in .governator/state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", ".local-state", "index.json")); err != nil {
		t.Fatalf("expected migrated index in .governator/.local-state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "worktrees", "task-T-001")); err != nil {
		t.Fatalf("expected task worktree moved to .governator/worktrees: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "worktrees", "merge-worktrees", "tmp", "touch.txt")); err != nil {
		t.Fatalf("expected merge worktree moved to .governator/worktrees: %v", err)
	}

	metaBytes, err := os.ReadFile(filepath.Join(repoRoot, ".governator", ".local-state", "meta", "T-001.json"))
	if err != nil {
		t.Fatalf("read migrated metadata: %v", err)
	}
	var metadata map[string]any
	if err := json.Unmarshal(metaBytes, &metadata); err != nil {
		t.Fatalf("decode migrated metadata: %v", err)
	}
	if got, _ := metadata["worktree_rel_path"].(string); got != ".governator/worktrees/task-T-001" {
		t.Fatalf("metadata worktree_rel_path = %q, want %q", got, ".governator/worktrees/task-T-001")
	}

	gitignoreBytes, err := os.ReadFile(filepath.Join(repoRoot, ".governator", ".gitignore"))
	if err != nil {
		t.Fatalf("read migrated gitignore: %v", err)
	}
	gitignore := string(gitignoreBytes)
	for _, rule := range []string{".local-state/*", "!.local-state/.keep", "worktrees/*", "!worktrees/.keep"} {
		if !strings.Contains(gitignore, rule) {
			t.Fatalf("expected gitignore to contain %q, got:\n%s", rule, gitignore)
		}
	}

	if _, err := os.Stat(filepath.Join(repoRoot, "_governator")); !os.IsNotExist(err) {
		t.Fatalf("legacy _governator directory should be removed; stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "state", "migrations", layoutMigrationID+".done")); err != nil {
		t.Fatalf("layout migration marker missing: %v", err)
	}
}

// TestApplyRepoMigrationsLayoutFailsOnConflictingFiles verifies conflicting target files fail explicitly.
func TestApplyRepoMigrationsLayoutFailsOnConflictingFiles(t *testing.T) {
	repoRoot := t.TempDir()

	legacyPath := filepath.Join(repoRoot, "_governator", "docs", "adr.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy docs dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy\n"), 0o644); err != nil {
		t.Fatalf("write legacy docs file: %v", err)
	}

	targetPath := filepath.Join(repoRoot, ".governator", "docs", "adr.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target docs dir: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("new-layout\n"), 0o644); err != nil {
		t.Fatalf("write target docs file: %v", err)
	}

	err := ApplyRepoMigrations(repoRoot, InitOptions{})
	if err == nil {
		t.Fatal("expected layout migration conflict error")
	}
	if !strings.Contains(err.Error(), "layout migration conflict") {
		t.Fatalf("expected layout migration conflict, got: %v", err)
	}

	markerPath := filepath.Join(repoRoot, repoDurableStateDir, "migrations", layoutMigrationID+".done")
	if _, statErr := os.Stat(markerPath); !os.IsNotExist(statErr) {
		t.Fatalf("layout migration marker should not exist on failure")
	}
}

// TestApplyRepoMigrationsCreatesCommitWhenApplied verifies successful migrations are committed atomically.
func TestApplyRepoMigrationsCreatesCommitWhenApplied(t *testing.T) {
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "_durable-state", "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir legacy migration dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator", "_durable-state", "config.json"), []byte("{\"workers\":{}}\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator", ".gitignore"), []byte("_local-state/*\n!_local-state/.keep\n"), 0o644); err != nil {
		t.Fatalf("write legacy gitignore: %v", err)
	}
	runGitCommand(t, repoRoot, "add", "_governator")
	runGitCommand(t, repoRoot, "commit", "-m", "seed legacy governator layout")

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	commitCount := runGitOutput(t, repoRoot, "rev-list", "--count", "HEAD")
	if commitCount != "5" {
		t.Fatalf("expected 5 commits total (2 seed + 3 migrations), got %s", commitCount)
	}

	subjects := runGitOutput(t, repoRoot, "log", "-3", "--pretty=%s")
	for _, migrationID := range []string{
		layoutMigrationID,
		conflictResolutionMigrationID,
		resetOpenTasksMigrationID,
	} {
		want := "governator: apply repo migration " + migrationID
		if !strings.Contains(subjects, want) {
			t.Fatalf("expected migration commit subject %q in recent commits, got:\n%s", want, subjects)
		}
	}
}

// TestApplyRepoMigrationsCommitsAllRepoChanges verifies migration commits include non-workspace paths.
func TestApplyRepoMigrationsCommitsAllRepoChanges(t *testing.T) {
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	if err := os.MkdirAll(filepath.Join(repoRoot, ".governator", "state", "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir migrations dir: %v", err)
	}
	for _, migrationID := range []string{layoutMigrationID, conflictResolutionMigrationID} {
		markerPath := filepath.Join(repoRoot, ".governator", "state", "migrations", migrationID+".done")
		if err := os.WriteFile(markerPath, []byte("ok\n"), 0o644); err != nil {
			t.Fatalf("write marker %s: %v", migrationID, err)
		}
	}

	taskID := "T-001-coder"
	indexPath := filepath.Join(repoRoot, ".governator", ".local-state", "index.json")
	if err := index.Save(indexPath, index.Index{
		SchemaVersion: 1,
		Digests:       index.Digests{PlanningDocs: map[string]string{}},
		Tasks: []index.Task{
			{
				ID:           taskID,
				Title:        "Example task",
				Path:         ".governator/tasks/T-001-coder.md",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateBlocked,
				Role:         "coder",
				Dependencies: []string{},
				Retries:      index.RetryPolicy{MaxAttempts: 3},
			},
		},
	}); err != nil {
		t.Fatalf("write index: %v", err)
	}
	notesPath := filepath.Join(repoRoot, "notes.txt")
	if err := os.WriteFile(notesPath, []byte("transient notes\n"), 0o644); err != nil {
		t.Fatalf("write notes: %v", err)
	}
	runGitCommand(t, repoRoot, "add", ".")
	runGitCommand(t, repoRoot, "commit", "-m", "seed migration fixture")

	if err := os.WriteFile(notesPath, []byte(""), 0o644); err != nil {
		t.Fatalf("truncate notes for deletion: %v", err)
	}
	if err := os.Remove(notesPath); err != nil {
		t.Fatalf("remove notes file: %v", err)
	}

	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	subject := runGitOutput(t, repoRoot, "log", "-1", "--pretty=%s")
	wantSubject := "governator: apply repo migration " + resetOpenTasksMigrationID
	if subject != wantSubject {
		t.Fatalf("unexpected commit subject: got %q want %q", subject, wantSubject)
	}

	tree := runGitOutput(t, repoRoot, "show", "--name-status", "--pretty=format:", "HEAD")
	if !strings.Contains(tree, "D\tnotes.txt") {
		t.Fatalf("expected deletion of notes.txt in migration commit, got:\n%s", tree)
	}
}

// TestApplyRepoMigrationsNoPendingDoesNotCreateCommit verifies no-op runs do not create extra commits.
func TestApplyRepoMigrationsNoPendingDoesNotCreateCommit(t *testing.T) {
	repoRoot := t.TempDir()
	initGitRepo(t, repoRoot)

	if err := os.MkdirAll(filepath.Join(repoRoot, repoDurableStateDir, "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir migrations dir: %v", err)
	}
	for _, migration := range repoMigrations {
		markerPath := filepath.Join(repoRoot, repoDurableStateDir, "migrations", migration.id+".done")
		if err := os.WriteFile(markerPath, []byte("ok\n"), 0o644); err != nil {
			t.Fatalf("write marker %s: %v", migration.id, err)
		}
	}
	runGitCommand(t, repoRoot, "add", ".governator")
	runGitCommand(t, repoRoot, "commit", "-m", "seed migration markers")

	before := runGitOutput(t, repoRoot, "rev-list", "--count", "HEAD")
	if err := ApplyRepoMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}
	after := runGitOutput(t, repoRoot, "rev-list", "--count", "HEAD")
	if before != after {
		t.Fatalf("expected commit count unchanged, before=%s after=%s", before, after)
	}
}

// TestStampExistingMigrationsWritesMarkers verifies that all known migrations get stamped.
func TestStampExistingMigrationsWritesMarkers(t *testing.T) {
	repoRoot := t.TempDir()
	if err := StampExistingMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("StampExistingMigrations: %v", err)
	}

	for _, migration := range repoMigrations {
		markerPath := filepath.Join(repoRoot, repoDurableStateDir, "migrations", migration.id+".done")
		data, err := os.ReadFile(markerPath)
		if err != nil {
			t.Fatalf("read marker for %s: %v", migration.id, err)
		}
		if string(data) != "ok\n" {
			t.Fatalf("marker content for %s = %q, want %q", migration.id, string(data), "ok\n")
		}
	}
}

// TestStampExistingMigrationsLeavesNoPending verifies that after stamping, no migrations are pending.
func TestStampExistingMigrationsLeavesNoPending(t *testing.T) {
	repoRoot := t.TempDir()
	if err := StampExistingMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("StampExistingMigrations: %v", err)
	}

	pending, err := PendingRepoMigrations(repoRoot)
	if err != nil {
		t.Fatalf("PendingRepoMigrations: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending migrations after stamp, got: %v", pending)
	}
}

// TestStampExistingMigrationsPreservesExistingMarkers verifies idempotency.
func TestStampExistingMigrationsPreservesExistingMarkers(t *testing.T) {
	repoRoot := t.TempDir()
	migrationsDir := filepath.Join(repoRoot, repoDurableStateDir, "migrations")
	if err := os.MkdirAll(migrationsDir, 0o755); err != nil {
		t.Fatalf("mkdir migrations: %v", err)
	}
	markerPath := filepath.Join(migrationsDir, conflictResolutionMigrationID+".done")
	custom := []byte("custom-marker\n")
	if err := os.WriteFile(markerPath, custom, 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	if err := StampExistingMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("StampExistingMigrations: %v", err)
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		t.Fatalf("read marker: %v", err)
	}
	if !bytes.Equal(data, custom) {
		t.Fatalf("existing marker was overwritten: got %q, want %q", string(data), string(custom))
	}
}

// TestStampExistingMigrationsRejectsEmptyRepoRoot verifies input validation.
func TestStampExistingMigrationsRejectsEmptyRepoRoot(t *testing.T) {
	err := StampExistingMigrations("", InitOptions{})
	if err == nil {
		t.Fatal("expected error for empty repo root")
	}
	if !strings.Contains(err.Error(), "repo root cannot be empty") {
		t.Fatalf("expected repo root validation error, got: %v", err)
	}
}

// TestInitFullLayoutThenStampLeavesNoPendingMigrations simulates the init flow.
func TestInitFullLayoutThenStampLeavesNoPendingMigrations(t *testing.T) {
	repoRoot := t.TempDir()
	if err := InitFullLayout(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("InitFullLayout: %v", err)
	}
	if err := StampExistingMigrations(repoRoot, InitOptions{}); err != nil {
		t.Fatalf("StampExistingMigrations: %v", err)
	}

	pending, err := PendingRepoMigrations(repoRoot)
	if err != nil {
		t.Fatalf("PendingRepoMigrations: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected no pending migrations after init + stamp, got: %v", pending)
	}
}

func initGitRepo(t *testing.T, repoRoot string) {
	t.Helper()
	runGitCommand(t, repoRoot, "init", "--initial-branch=main")
	runGitCommand(t, repoRoot, "config", "user.name", "Governator Test")
	runGitCommand(t, repoRoot, "config", "user.email", "test@example.com")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatalf("write README: %v", err)
	}
	runGitCommand(t, repoRoot, "add", "README.md")
	runGitCommand(t, repoRoot, "commit", "-m", "init")
}

func runGitCommand(t *testing.T, repoRoot string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, string(output))
	}
}

func runGitOutput(t *testing.T, repoRoot string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v: %s", strings.Join(args, " "), err, string(output))
	}
	return strings.TrimSpace(string(output))
}

func branchExists(t *testing.T, repoRoot string, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoRoot
	err := cmd.Run()
	return err == nil
}
