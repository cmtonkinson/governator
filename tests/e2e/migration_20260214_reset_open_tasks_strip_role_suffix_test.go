// Package test provides end-to-end coverage for destructive task reset migration behavior.
package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/inflight"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

const (
	e2eResetMigrationID     = "20260214_reset_open_tasks_strip_role_suffix"
	e2eResetMigrationMarker = ".governator/state/migrations/20260214_reset_open_tasks_strip_role_suffix.done"
)

// TestE2EMigrationResetOpenTasksHappyPath verifies the destructive migration resets open tasks, strips suffixes, and purges branch/worktree state.
func TestE2EMigrationResetOpenTasksHappyPath(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root
	TrackE2ERepo(t, repoRoot)

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
		"010-api-client-architect.md",
		"011-api-tests-default.md",
		"012-release-notes.md",
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
				State: index.TaskState("governator_planning_not_started"),
				Role:  "planner",
			},
			{
				ID:           "010-api-client-architect",
				Path:         ".governator/tasks/010-api-client-architect.md",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateTriaged,
				Role:         "architect",
				Dependencies: []string{},
			},
			{
				ID:            "011-api-tests-default",
				Path:          ".governator/tasks/011-api-tests-default.md",
				Kind:          index.TaskKindExecution,
				State:         index.TaskStateBlocked,
				Role:          "default",
				Dependencies:  []string{"010-api-client-architect"},
				BlockedReason: "failure",
			},
			{
				ID:           "012-release-notes",
				Path:         ".governator/tasks/012-release-notes.md",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateMerged,
				Role:         "default",
				Dependencies: []string{"011-api-tests-default"},
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
		"010-api-client-architect": {ID: "010-api-client-architect"},
		"011-api-tests-default":    {ID: "011-api-tests-default"},
	}); err != nil {
		t.Fatalf("seed in-flight set: %v", err)
	}

	repo.RunGit(t, "branch", "010-api-client-architect")
	worktreePath := filepath.Join(repoRoot, ".governator", "worktrees", "task-010-api-client-architect")
	repo.RunGit(t, "worktree", "add", worktreePath, "010-api-client-architect")
	repo.RunGit(t, "branch", "task-011-api-tests-default")

	if err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	updated, err := index.Load(indexPath)
	if err != nil {
		t.Fatalf("load migrated index: %v", err)
	}
	byID := map[string]index.Task{}
	for _, task := range updated.Tasks {
		byID[task.ID] = task
	}
	if byID["planning"].State != index.TaskState("governator_planning_not_started") {
		t.Fatalf("planning state changed: got %s", byID["planning"].State)
	}

	task010 := byID["010-api-client"]
	if task010.State != index.TaskStateBacklog {
		t.Fatalf("task010 state = %s, want backlog", task010.State)
	}
	if task010.Path != ".governator/tasks/010-api-client.md" {
		t.Fatalf("task010 path = %s, want .governator/tasks/010-api-client.md", task010.Path)
	}

	task011 := byID["011-api-tests"]
	if task011.State != index.TaskStateBacklog {
		t.Fatalf("task011 state = %s, want backlog", task011.State)
	}
	if task011.Role != "" {
		t.Fatalf("task011 role = %q, want empty", task011.Role)
	}
	if len(task011.Dependencies) != 1 || task011.Dependencies[0] != "010-api-client" {
		t.Fatalf("task011 dependencies = %#v, want [010-api-client]", task011.Dependencies)
	}

	task012 := byID["012-release-notes"]
	if len(task012.Dependencies) != 1 || task012.Dependencies[0] != "011-api-tests" {
		t.Fatalf("task012 dependencies = %#v, want [011-api-tests]", task012.Dependencies)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "tasks", "010-api-client.md")); err != nil {
		t.Fatalf("renamed task file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "tasks", "010-api-client-architect.md")); !os.IsNotExist(err) {
		t.Fatalf("old suffixed task file should be gone, stat err=%v", err)
	}

	inFlightAfter, err := store.Load()
	if err != nil {
		t.Fatalf("load in-flight after migration: %v", err)
	}
	if len(inFlightAfter) != 0 {
		t.Fatalf("expected cleared in-flight set, got %#v", inFlightAfter)
	}

	if _, err := os.Stat(worktreePath); !os.IsNotExist(err) {
		t.Fatalf("worktree should be removed, stat err=%v", err)
	}
	if e2eBranchExists(t, repoRoot, "010-api-client-architect") {
		t.Fatalf("task branch should be deleted")
	}
	if e2eBranchExists(t, repoRoot, "task-011-api-tests-default") {
		t.Fatalf("legacy task-* branch should be deleted")
	}
	if _, err := os.Stat(filepath.Join(repoRoot, e2eResetMigrationMarker)); err != nil {
		t.Fatalf("migration marker missing: %v", err)
	}
}

// TestE2EMigrationResetOpenTasksCollisionFailure verifies collisions fail explicitly and leave no completion marker.
func TestE2EMigrationResetOpenTasksCollisionFailure(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root
	TrackE2ERepo(t, repoRoot)

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
	if err := os.WriteFile(filepath.Join(tasksDir, "020-dup-architect.md"), []byte("# dup 1\n"), 0o644); err != nil {
		t.Fatalf("write task 1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tasksDir, "020-dup-planner.md"), []byte("# dup 2\n"), 0o644); err != nil {
		t.Fatalf("write task 2: %v", err)
	}

	idx := index.Index{
		SchemaVersion: 1,
		Digests:       index.Digests{PlanningDocs: map[string]string{}},
		Tasks: []index.Task{
			{ID: "020-dup-architect", Path: ".governator/tasks/020-dup-architect.md", Kind: index.TaskKindExecution, State: index.TaskStateTriaged},
			{ID: "020-dup-planner", Path: ".governator/tasks/020-dup-planner.md", Kind: index.TaskKindExecution, State: index.TaskStateTriaged},
		},
	}
	indexPath := filepath.Join(repoRoot, ".governator", ".local-state", "index.json")
	if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
		t.Fatalf("mkdir index dir: %v", err)
	}
	if err := index.Save(indexPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{})
	if err == nil {
		t.Fatal("expected migration collision error")
	}
	if !strings.Contains(err.Error(), "rename collision") {
		t.Fatalf("expected rename collision error, got: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(repoRoot, e2eResetMigrationMarker)); !os.IsNotExist(statErr) {
		t.Fatalf("migration marker should not exist after collision")
	}
}

func e2eBranchExists(t *testing.T, repoRoot string, branch string) bool {
	t.Helper()
	cmd := exec.Command("git", "show-ref", "--verify", "--quiet", "refs/heads/"+branch)
	cmd.Dir = repoRoot
	return cmd.Run() == nil
}
