// Package test provides end-to-end coverage for legacy workspace layout migration behavior.
package test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/config"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

const (
	e2eLayoutMigrationID     = "20260216_migrate_workspace_layout"
	e2eLayoutMigrationMarker = ".governator/state/migrations/20260216_migrate_workspace_layout.done"
)

// TestE2EMigrationWorkspaceLayoutHappyPath verifies legacy _governator state is migrated to .governator layout.
func TestE2EMigrationWorkspaceLayoutHappyPath(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root
	TrackE2ERepo(t, repoRoot)

	if err := os.RemoveAll(filepath.Join(repoRoot, ".governator")); err != nil {
		t.Fatalf("remove bootstrap .governator: %v", err)
	}

	legacyRoot := filepath.Join(repoRoot, "_governator")
	if err := os.MkdirAll(filepath.Join(legacyRoot, "_durable-state", "migrations"), 0o755); err != nil {
		t.Fatalf("mkdir legacy durable-state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "_durable-state", "config.json"), []byte("{\"timeouts\":{\"worker_seconds\":900}}\n"), 0o644); err != nil {
		t.Fatalf("write legacy config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "_durable-state", "migrations", "20260209_add_conflict_resolution_prompt.done"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write legacy marker 20260209: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "_durable-state", "migrations", "20260214_reset_open_tasks_strip_role_suffix.done"), []byte("ok\n"), 0o644); err != nil {
		t.Fatalf("write legacy marker 20260214: %v", err)
	}

	legacyLocal := filepath.Join(legacyRoot, "_local-state")
	if err := os.MkdirAll(filepath.Join(legacyLocal, "meta"), 0o755); err != nil {
		t.Fatalf("mkdir legacy meta: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyLocal, "index.json"), []byte("{\"schema_version\":1,\"tasks\":[],\"digests\":{\"planning_docs\":{}}}\n"), 0o644); err != nil {
		t.Fatalf("write legacy index: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(legacyLocal, "task-T-900"), 0o755); err != nil {
		t.Fatalf("mkdir legacy task worktree: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyLocal, "meta", "T-900.json"), []byte("{\"worktree_rel_path\":\"_governator/_local-state/task-T-900\"}\n"), 0o644); err != nil {
		t.Fatalf("write legacy metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, ".gitignore"), []byte("_local-state/*\n!_local-state/.keep\n"), 0o644); err != nil {
		t.Fatalf("write legacy gitignore: %v", err)
	}

	if err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{}); err != nil {
		t.Fatalf("ApplyRepoMigrations: %v", err)
	}

	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "state", "config.json")); err != nil {
		t.Fatalf("expected config moved to .governator/state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", ".local-state", "index.json")); err != nil {
		t.Fatalf("expected index moved to .governator/.local-state: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, ".governator", "worktrees", "task-T-900")); err != nil {
		t.Fatalf("expected task worktree moved to .governator/worktrees: %v", err)
	}

	metaPath := filepath.Join(repoRoot, ".governator", ".local-state", "meta", "T-900.json")
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read migrated metadata: %v", err)
	}
	var meta map[string]any
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		t.Fatalf("decode migrated metadata: %v", err)
	}
	if got, _ := meta["worktree_rel_path"].(string); got != ".governator/worktrees/task-T-900" {
		t.Fatalf("worktree_rel_path = %q, want %q", got, ".governator/worktrees/task-T-900")
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
		t.Fatalf("legacy _governator should be removed; stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoRoot, e2eLayoutMigrationMarker)); err != nil {
		t.Fatalf("layout migration marker missing: %v", err)
	}
}

// TestE2EMigrationWorkspaceLayoutConflictFailure verifies migration fails on conflicting destination content.
func TestE2EMigrationWorkspaceLayoutConflictFailure(t *testing.T) {
	repo := testrepos.New(t)
	repoRoot := repo.Root
	TrackE2ERepo(t, repoRoot)

	legacyPath := filepath.Join(repoRoot, "_governator", "docs", "layout.md")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		t.Fatalf("mkdir legacy docs dir: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte("legacy layout\n"), 0o644); err != nil {
		t.Fatalf("write legacy docs file: %v", err)
	}

	targetPath := filepath.Join(repoRoot, ".governator", "docs", "layout.md")
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		t.Fatalf("mkdir target docs dir: %v", err)
	}
	if err := os.WriteFile(targetPath, []byte("new layout\n"), 0o644); err != nil {
		t.Fatalf("write target docs file: %v", err)
	}

	err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{})
	if err == nil {
		t.Fatal("expected layout migration conflict failure")
	}
	if !strings.Contains(err.Error(), e2eLayoutMigrationID) {
		t.Fatalf("expected migration ID in error, got: %v", err)
	}
	if !strings.Contains(err.Error(), "layout migration conflict") {
		t.Fatalf("expected conflict error, got: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(repoRoot, e2eLayoutMigrationMarker)); !os.IsNotExist(statErr) {
		t.Fatalf("layout migration marker should be absent after failure")
	}
}
