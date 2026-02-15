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
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "roles"), 0o755); err != nil {
		t.Fatalf("mkdir roles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator", "roles", "architect.md"), []byte("# Architect\n"), 0o644); err != nil {
		t.Fatalf("write architect role: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator", "roles", "default.md"), []byte("# Default\n"), 0o644); err != nil {
		t.Fatalf("write default role: %v", err)
	}

	tasksDir := filepath.Join(repoRoot, "_governator", "tasks")
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
				Path:  "_governator/planning.json",
				Kind:  index.TaskKindPlanning,
				State: index.TaskState("governator_planning_complete"),
				Role:  "planner",
			},
			{
				ID:           "001-build-api-architect",
				Path:         "_governator/tasks/001-build-api-architect.md",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateTriaged,
				Role:         "architect",
				Dependencies: []string{},
			},
			{
				ID:            "002-test-api-default",
				Path:          "_governator/tasks/002-test-api-default.md",
				Kind:          index.TaskKindExecution,
				State:         index.TaskStateBlocked,
				Role:          "default",
				Dependencies:  []string{"001-build-api-architect"},
				Attempts:      index.AttemptCounters{Total: 2, Failed: 2},
				BlockedReason: "timed out",
			},
			{
				ID:           "003-release-notes",
				Path:         "_governator/tasks/003-release-notes.md",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateMerged,
				Role:         "default",
				Dependencies: []string{"002-test-api-default"},
			},
		},
	}
	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
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
	worktreePath := filepath.Join(repoRoot, "_governator", "_local-state", "task-001-build-api-architect")
	runGitCommand(t, repoRoot, "worktree", "add", worktreePath, "001-build-api-architect")
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "_local-state", "meta"), 0o755); err != nil {
		t.Fatalf("mkdir meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator", "_local-state", "meta", "001-build-api-architect.json"), []byte("{}\n"), 0o644); err != nil {
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
	if taskA.Path != "_governator/tasks/001-build-api.md" {
		t.Fatalf("taskA path = %s, want _governator/tasks/001-build-api.md", taskA.Path)
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

	if _, err := os.Stat(filepath.Join(repoRoot, "_governator", "tasks", "001-build-api.md")); err != nil {
		t.Fatalf("renamed file 001-build-api.md missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, "_governator", "tasks", "001-build-api-architect.md")); !os.IsNotExist(err) {
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
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "prompts"), 0o755); err != nil {
		t.Fatalf("mkdir prompts: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "roles"), 0o755); err != nil {
		t.Fatalf("mkdir roles: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator", "roles", "architect.md"), []byte("# Architect\n"), 0o644); err != nil {
		t.Fatalf("write architect role: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "_governator", "roles", "planner.md"), []byte("# Planner\n"), 0o644); err != nil {
		t.Fatalf("write planner role: %v", err)
	}

	tasksDir := filepath.Join(repoRoot, "_governator", "tasks")
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
			{ID: "001-feature-architect", Path: "_governator/tasks/001-feature-architect.md", Kind: index.TaskKindExecution, State: index.TaskStateTriaged},
			{ID: "001-feature-planner", Path: "_governator/tasks/001-feature-planner.md", Kind: index.TaskKindExecution, State: index.TaskStateTriaged},
		},
	}
	indexPath := filepath.Join(repoRoot, "_governator", "_local-state", "index.json")
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

func branchExists(t *testing.T, repoRoot string, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoRoot
	err := cmd.Run()
	return err == nil
}
