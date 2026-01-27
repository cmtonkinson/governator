// Tests for Go-owned git finalization after worker completion.
package run

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/roles"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestFinalizeStageSuccessCommitsChanges ensures Governator captures status and commits.
func TestFinalizeStageSuccessCommitsChanges(t *testing.T) {
	t.Parallel()
	repo := testrepos.New(t)
	worktreePath := repo.Root
	configureLocalStateIgnore(t, repo)

	workerStateDir := filepath.Join(worktreePath, "_governator", "_local-state", "worker-test")
	if err := os.MkdirAll(workerStateDir, 0o755); err != nil {
		t.Fatalf("mkdir worker state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerStateDir, stdoutLogFileName), []byte("worker stdout\n"), 0o644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}

	changePath := filepath.Join(worktreePath, "worker-output.txt")
	if err := os.WriteFile(changePath, []byte("change\n"), 0o644); err != nil {
		t.Fatalf("write change: %v", err)
	}

	task := index.Task{ID: "T-001", Title: "Demo task"}
	result, err := finalizeStageSuccess(worktreePath, workerStateDir, task, roles.StageWork)
	if err != nil {
		t.Fatalf("finalize stage: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}
	if result.NewState != index.TaskStateImplemented {
		t.Fatalf("new state = %q, want %q", result.NewState, index.TaskStateImplemented)
	}

	statusPath := filepath.Join(workerStateDir, gitChangesFileName)
	statusContent, err := os.ReadFile(statusPath)
	if err != nil {
		t.Fatalf("read git-changes: %v", err)
	}
	if !strings.Contains(string(statusContent), "worker-output.txt") {
		t.Fatalf("git-changes.txt missing change: %s", string(statusContent))
	}

	logOutput := runGitLog(t, worktreePath)
	if !strings.Contains(logOutput, "[implemented] Demo task") {
		t.Fatalf("unexpected commit subject: %s", logOutput)
	}
	if !strings.Contains(logOutput, "worker stdout") {
		t.Fatalf("stdout log missing from commit body: %s", logOutput)
	}
}

// TestFinalizeStageSuccessNoChanges still writes status output and succeeds cleanly.
func TestFinalizeStageSuccessNoChanges(t *testing.T) {
	t.Parallel()
	repo := testrepos.New(t)
	worktreePath := repo.Root
	configureLocalStateIgnore(t, repo)

	workerStateDir := filepath.Join(worktreePath, "_governator", "_local-state", "worker-test")
	if err := os.MkdirAll(workerStateDir, 0o755); err != nil {
		t.Fatalf("mkdir worker state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workerStateDir, stdoutLogFileName), []byte("no changes\n"), 0o644); err != nil {
		t.Fatalf("write stdout log: %v", err)
	}

	task := index.Task{ID: "T-002", Title: "No-op task"}
	result, err := finalizeStageSuccess(worktreePath, workerStateDir, task, roles.StageWork)
	if err != nil {
		t.Fatalf("finalize stage: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got %#v", result)
	}

	statusPath := filepath.Join(workerStateDir, gitChangesFileName)
	if _, err := os.Stat(statusPath); err != nil {
		t.Fatalf("git-changes.txt missing: %v", err)
	}
}

// runGitLog returns the latest commit message body in the worktree.
func runGitLog(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "log", "-1", "--pretty=%B")
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return string(output)
}

// configureLocalStateIgnore mimics the production layout's local-state gitignore.
func configureLocalStateIgnore(t *testing.T, repo *testrepos.TempRepo) {
	t.Helper()
	ignorePath := filepath.Join(repo.Root, "_governator", ".gitignore")
	if err := os.MkdirAll(filepath.Dir(ignorePath), 0o755); err != nil {
		t.Fatalf("mkdir _governator: %v", err)
	}
	content := "_local-state/*\n!_local-state/.keep\n"
	if err := os.WriteFile(ignorePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write .gitignore: %v", err)
	}
	repo.RunGit(t, "add", filepath.Join("_governator", ".gitignore"))
	repo.RunGit(t, "commit", "-m", "Ignore local state")
}
