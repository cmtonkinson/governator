package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cmtonkinson/governator/internal/config"
)

const usageMessage = "USAGE:\n    governator [global options] <command> [command options]"

func TestCLICommands(t *testing.T) {
	// Build the CLI binary for testing
	binaryPath := filepath.Join(t.TempDir(), "governator-test")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	tests := []struct {
		name           string
		args           []string
		expectedExit   int
		expectedOutput string
		expectedError  string
	}{
		{
			name:          "no arguments shows usage",
			args:          []string{},
			expectedExit:  2,
			expectedError: usageMessage,
		},
		{
			name:          "unknown command shows usage",
			args:          []string{"unknown"},
			expectedExit:  2,
			expectedError: usageMessage,
		},
		{
			name:           "version flag",
			args:           []string{"--version"},
			expectedExit:   0,
			expectedOutput: "version=dev commit=unknown built_at=unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.Command(binaryPath, tt.args...)
			output, err := cmd.CombinedOutput()

			// Check exit code
			var exitCode int
			if err != nil {
				if exitError, ok := err.(*exec.ExitError); ok {
					exitCode = exitError.ExitCode()
				} else {
					t.Fatalf("Unexpected error type: %v", err)
				}
			}

			if exitCode != tt.expectedExit {
				t.Errorf("Expected exit code %d, got %d", tt.expectedExit, exitCode)
			}

			outputStr := strings.TrimSpace(string(output))

			// Check expected output
			if tt.expectedOutput != "" && !strings.Contains(outputStr, tt.expectedOutput) {
				t.Errorf("Expected output to contain %q, got %q", tt.expectedOutput, outputStr)
			}

			// Check expected error
			if tt.expectedError != "" && !strings.Contains(outputStr, tt.expectedError) {
				t.Errorf("Expected error to contain %q, got %q", tt.expectedError, outputStr)
			}
		})
	}
}

func TestVersionCommandWithMetadata(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "governator-version-metadata")
	ldflags := "-X github.com/cmtonkinson/governator/internal/buildinfo.Version=1.2.3 -X github.com/cmtonkinson/governator/internal/buildinfo.Commit=8d3f2a1 -X github.com/cmtonkinson/governator/internal/buildinfo.BuiltAt=2025-02-14T09:30:00Z"
	cmd := exec.Command("go", "build", "-ldflags", ldflags, "-o", binaryPath, ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary with metadata: %v", err)
	}

	output, err := exec.Command(binaryPath, "--version").CombinedOutput()
	if err != nil {
		t.Fatalf("Version flag failed: %v, output: %s", err, output)
	}

	outputStr := strings.TrimSpace(string(output))
	expected := "version=1.2.3 commit=8d3f2a1 built_at=2025-02-14T09:30:00Z"
	if outputStr != expected {
		t.Fatalf("Expected %q, got %q", expected, outputStr)
	}
}

func TestConfirmPendingMigrations(t *testing.T) {
	t.Run("returns true without output when no migrations are pending", func(t *testing.T) {
		repoRoot := t.TempDir()
		if err := os.MkdirAll(filepath.Join(repoRoot, "_governator", "prompts"), 0o755); err != nil {
			t.Fatalf("mkdir prompts: %v", err)
		}
		if err := config.ApplyRepoMigrations(repoRoot, config.InitOptions{}); err != nil {
			t.Fatalf("apply repo migrations: %v", err)
		}

		var out bytes.Buffer
		ok, err := confirmPendingMigrations(repoRoot, strings.NewReader(""), &out)
		if err != nil {
			t.Fatalf("confirmPendingMigrations: %v", err)
		}
		if !ok {
			t.Fatal("expected confirmation to proceed when nothing is pending")
		}
		if out.Len() != 0 {
			t.Fatalf("expected no output when no migrations are pending, got: %q", out.String())
		}
	})

	t.Run("prompts and proceeds on default confirmation", func(t *testing.T) {
		repoRoot := t.TempDir()
		var out bytes.Buffer

		ok, err := confirmPendingMigrations(repoRoot, strings.NewReader("\n"), &out)
		if err != nil {
			t.Fatalf("confirmPendingMigrations: %v", err)
		}
		if !ok {
			t.Fatal("expected confirmation to proceed on default input")
		}

		output := out.String()
		if !strings.Contains(output, "Identified pending migrations!") {
			t.Fatalf("expected pending migration banner, got: %q", output)
		}
		if !strings.Contains(output, "20260209_add_conflict_resolution_prompt") {
			t.Fatalf("expected migration id in prompt output, got: %q", output)
		}
		if !strings.Contains(output, "Apply these? ([y]/n)") {
			t.Fatalf("expected apply confirmation prompt, got: %q", output)
		}
	})

	t.Run("prompts and aborts on explicit no", func(t *testing.T) {
		repoRoot := t.TempDir()
		var out bytes.Buffer

		ok, err := confirmPendingMigrations(repoRoot, strings.NewReader("n\n"), &out)
		if err != nil {
			t.Fatalf("confirmPendingMigrations: %v", err)
		}
		if ok {
			t.Fatal("expected confirmation to abort on explicit no")
		}

		output := out.String()
		if !strings.Contains(output, "Identified pending migrations!") {
			t.Fatalf("expected pending migration banner, got: %q", output)
		}
		if !strings.Contains(output, "Apply these? ([y]/n)") {
			t.Fatalf("expected apply confirmation prompt, got: %q", output)
		}
	})
}

func TestInitCommand(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "governator-init-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Build the CLI binary for testing
	binaryPath := filepath.Join(t.TempDir(), "governator-test")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	// Change to temp directory and run init
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	// Create a git repo in temp dir (required for init to work)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Initialize git repo
	gitCmd := exec.Command("git", "init")
	if err := gitCmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	// Run init command
	cmd = exec.Command(binaryPath, "init")
	output, err := cmd.CombinedOutput()

	if err != nil {
		t.Fatalf("Init command failed: %v, output: %s", err, output)
	}

	outputStr := strings.TrimSpace(string(output))
	if outputStr != "init ok" {
		t.Errorf("Expected 'init ok', got %q", outputStr)
	}

	// Check that directories were created
	expectedDirs := []string{
		"_governator/_durable-state",
		"_governator/_durable-state/migrations",
		"_governator/docs",
		"_governator/docs/adr",
		"_governator/roles",
		"_governator/custom-prompts",
		"_governator/prompts",
		"_governator/templates",
		"_governator/reasoning",
		"_governator/_local-state",
	}

	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory %s was not created", dir)
		}
	}

	// Check that config file was created
	configPath := filepath.Join("_governator", "_durable-state", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Expected config.json was not created")
	}

	gitignorePath := filepath.Join("_governator", ".gitignore")
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		t.Error("Expected _governator/.gitignore was not created")
	}

	customPromptPath := filepath.Join("_governator", "custom-prompts", "_global.md")
	if _, err := os.Stat(customPromptPath); os.IsNotExist(err) {
		t.Error("Expected custom prompt _global.md was not created")
	}

	gitLogCmd := exec.Command("git", "-C", tempDir, "log", "-1", "--pretty=%B")
	logOut, err := gitLogCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log failed: %v, output: %s", err, logOut)
	}
	if strings.TrimSpace(string(logOut)) != "Governator initialized" {
		t.Errorf("expected commit message %q, got %q", "Governator initialized", strings.TrimSpace(string(logOut)))
	}
}

func TestStatusCommand(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "governator-status-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Build the CLI binary for testing
	binaryPath := filepath.Join(t.TempDir(), "governator-test")
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	// Change to temp directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	// Create a git repo in temp dir (required for repo discovery)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}

	// Initialize git repo
	gitCmd := exec.Command("git", "init")
	if err := gitCmd.Run(); err != nil {
		t.Fatalf("Failed to init git repo: %v", err)
	}

	t.Run("status without index fails", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "status")
		output, err := cmd.CombinedOutput()

		var exitCode int
		if err != nil {
			if exitError, ok := err.(*exec.ExitError); ok {
				exitCode = exitError.ExitCode()
			} else {
				t.Fatalf("Unexpected error type: %v", err)
			}
		}

		if exitCode != 1 {
			t.Errorf("Expected exit code 1, got %d", exitCode)
		}

		outputStr := strings.TrimSpace(string(output))
		if !strings.Contains(outputStr, "load task index") {
			t.Errorf("Expected error about loading task index, got %q", outputStr)
		}
	})

	// Initialize the repo structure first
	initCmd := exec.Command(binaryPath, "init")
	initCmd.Dir = tempDir
	if output, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("Init command failed: %v, output: %s", err, output)
	}

	t.Run("status with empty index", func(t *testing.T) {
		// The init command already created the index structure, just run status
		cmd := exec.Command(binaryPath, "status")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Fatalf("Status command failed: %v, output: %s", err, output)
		}

		outputStr := strings.TrimSpace(string(output))
		if !strings.Contains(outputStr, "backlog=0") || !strings.Contains(outputStr, "merged=0") || !strings.Contains(outputStr, "in-progress=0") {
			t.Errorf("Expected counts in output, got %q", outputStr)
		}
	})

	t.Run("status with populated index", func(t *testing.T) {
		// Create a populated task index
		indexPath := filepath.Join(tempDir, "_governator", "_local-state", "index.json")
		populatedIndex := `{
				"schema_version": 1,
				"tasks": [
					{"id": "001-done-task", "kind": "execution", "state": "done"},
					{"id": "002-done-task", "kind": "execution", "state": "done"},
					{"id": "003-open-task", "kind": "execution", "state": "open"},
					{"id": "004-open-task", "kind": "execution", "state": "open"},
					{"id": "005-blocked-task", "kind": "execution", "state": "blocked"},
					{"id": "006-worked-task", "kind": "execution", "state": "worked"},
					{"id": "007-conflict-task", "kind": "execution", "state": "conflict"}
				]
			}`
		if err := os.WriteFile(indexPath, []byte(populatedIndex), 0644); err != nil {
			t.Fatalf("Failed to write populated index: %v", err)
		}

		cmd := exec.Command(binaryPath, "status")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()

		if err != nil {
			t.Fatalf("Status command failed: %v, output: %s", err, output)
		}

		outputStr := strings.TrimSpace(string(output))
		if !strings.Contains(outputStr, "backlog=0 merged=2 in-progress=5") {
			t.Fatalf("expected counts line in output, got %q", outputStr)
		}
		if !strings.Contains(outputStr, "id             state") {
			t.Fatalf("expected table header, got %q", outputStr)
		}
		// Check for numeric prefixes (not full task IDs)
		for _, id := range []string{"003", "004", "005", "006", "007"} {
			if !strings.Contains(outputStr, id) {
				t.Fatalf("expected task %s in output, got %q", id, outputStr)
			}
		}
	})
}

func TestWhyCommand(t *testing.T) {
	binaryPath := filepath.Join(t.TempDir(), "governator-test")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("Failed to build CLI binary: %v", err)
	}

	tempDir := t.TempDir()
	gitInitCmd := exec.Command("git", "init")
	gitInitCmd.Dir = tempDir
	if out, err := gitInitCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v, output: %s", err, out)
	}

	logPath := filepath.Join(tempDir, "_governator", "_local-state", "supervisor", "supervisor.log")
	if err := os.MkdirAll(filepath.Dir(logPath), 0o755); err != nil {
		t.Fatalf("mkdir log dir: %v", err)
	}
	lines := make([]string, 30)
	for i := 1; i <= 30; i++ {
		lines[i-1] = fmt.Sprintf("line-%02d", i)
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatalf("write supervisor log: %v", err)
	}

	t.Run("default line count is 20", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "why")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("why failed: %v, output: %s", err, output)
		}

		got := strings.TrimSpace(string(output))
		want := "=== Supervisor 0 (unknown) last 20 lines ===\n" + strings.Join(lines[10:], "\n")
		if got != want {
			t.Fatalf("unexpected output\nwant:\n%s\n\ngot:\n%s", want, got)
		}
	})

	t.Run("custom supervisor line count with -s", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "why", "-s", "5")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("why -s 5 failed: %v, output: %s", err, output)
		}

		got := strings.TrimSpace(string(output))
		want := "=== Supervisor 0 (unknown) last 5 lines ===\n" + strings.Join(lines[25:], "\n")
		if got != want {
			t.Fatalf("unexpected output\nwant:\n%s\n\ngot:\n%s", want, got)
		}
	})

	t.Run("rejects non-positive -s", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "why", "-s", "0")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected failure for non-positive -s, output: %s", output)
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected ExitError, got %T", err)
		}
		if exitErr.ExitCode() != 2 {
			t.Fatalf("exit code = %d, want 2", exitErr.ExitCode())
		}
		if !strings.Contains(string(output), "must be a positive integer") {
			t.Fatalf("expected validation error, got: %s", output)
		}
	})

	t.Run("rejects non-positive -t", func(t *testing.T) {
		cmd := exec.Command(binaryPath, "why", "-t", "0")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()
		if err == nil {
			t.Fatalf("expected failure for non-positive -t, output: %s", output)
		}
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("expected ExitError, got %T", err)
		}
		if exitErr.ExitCode() != 2 {
			t.Fatalf("exit code = %d, want 2", exitErr.ExitCode())
		}
		if !strings.Contains(string(output), "must be a positive integer") {
			t.Fatalf("expected validation error, got: %s", output)
		}
	})

	t.Run("shows blocked and failed task sections using most recent stdout", func(t *testing.T) {
		blockedID := "T-BLOCKED-001"
		failedID := "T-FAILED-001"
		openID := "T-OPEN-001"

		indexPath := filepath.Join(tempDir, "_governator", "_local-state", "index.json")
		indexJSON := fmt.Sprintf(`{
  "schema_version": 1,
  "tasks": [
    {
      "id": %q,
      "path": "_governator/tasks/%s.md",
      "kind": "execution",
      "state": "blocked",
      "role": "default",
      "dependencies": [],
      "retries": {"max_attempts": 2},
      "attempts": {"total": 1, "failed": 0},
      "order": 1,
      "overlap": []
    },
    {
      "id": %q,
      "path": "_governator/tasks/%s.md",
      "kind": "execution",
      "state": "triaged",
      "role": "default",
      "dependencies": [],
      "retries": {"max_attempts": 2},
      "attempts": {"total": 2, "failed": 1},
      "order": 2,
      "overlap": []
    },
    {
      "id": %q,
      "path": "_governator/tasks/%s.md",
      "kind": "execution",
      "state": "triaged",
      "role": "default",
      "dependencies": [],
      "retries": {"max_attempts": 2},
      "attempts": {"total": 0, "failed": 0},
      "order": 3,
      "overlap": []
    }
  ]
}`, blockedID, blockedID, failedID, failedID, openID, openID)
		if err := os.WriteFile(indexPath, []byte(indexJSON), 0o644); err != nil {
			t.Fatalf("write index: %v", err)
		}

		blockedRoot := filepath.Join(tempDir, "_governator", "_local-state", "task-"+blockedID, "_governator", "_local-state")
		blockedOldDir := filepath.Join(blockedRoot, "worker-1-work-default")
		blockedNewDir := filepath.Join(blockedRoot, "worker-2-work-default")
		if err := os.MkdirAll(blockedOldDir, 0o755); err != nil {
			t.Fatalf("mkdir blocked old dir: %v", err)
		}
		if err := os.MkdirAll(blockedNewDir, 0o755); err != nil {
			t.Fatalf("mkdir blocked new dir: %v", err)
		}
		blockedOldLog := filepath.Join(blockedOldDir, "stdout.log")
		blockedNewLog := filepath.Join(blockedNewDir, "stdout.log")
		if err := os.WriteFile(blockedOldLog, []byte("old-1\nold-2\n"), 0o644); err != nil {
			t.Fatalf("write blocked old log: %v", err)
		}
		if err := os.WriteFile(blockedNewLog, []byte("new-1\nnew-2\nnew-3\nnew-4\n"), 0o644); err != nil {
			t.Fatalf("write blocked new log: %v", err)
		}
		oldTime := time.Unix(1000, 0)
		newTime := time.Unix(2000, 0)
		if err := os.Chtimes(blockedOldLog, oldTime, oldTime); err != nil {
			t.Fatalf("chtimes old log: %v", err)
		}
		if err := os.Chtimes(blockedNewLog, newTime, newTime); err != nil {
			t.Fatalf("chtimes new log: %v", err)
		}

		failedRoot := filepath.Join(tempDir, "_governator", "_local-state", "task-"+failedID, "_governator", "_local-state")
		failedDir := filepath.Join(failedRoot, "worker-1-test-default")
		if err := os.MkdirAll(failedDir, 0o755); err != nil {
			t.Fatalf("mkdir failed dir: %v", err)
		}
		failedLog := filepath.Join(failedDir, "stdout.log")
		if err := os.WriteFile(failedLog, []byte("f-1\nf-2\nf-3\n"), 0o644); err != nil {
			t.Fatalf("write failed log: %v", err)
		}

		cmd := exec.Command(binaryPath, "why", "-s", "2", "-t", "3")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("why -s 2 -t 3 failed: %v, output: %s", err, output)
		}
		got := string(output)

		if strings.Count(got, "=== ") != 3 {
			t.Fatalf("expected exactly 3 sections (supervisor + blocked + failed), got output:\n%s", got)
		}
		if !strings.Contains(got, "=== Supervisor 0 (unknown) last 2 lines ===\nline-29\nline-30\n") {
			t.Fatalf("missing supervisor section in output:\n%s", got)
		}
		if !strings.Contains(got, "=== Task "+blockedID+" (blocked) last 3 lines from _governator/_local-state/task-"+blockedID+"/_governator/_local-state/worker-2-work-default/stdout.log ===\nnew-2\nnew-3\nnew-4\n") {
			t.Fatalf("missing blocked section with most recent stdout log in output:\n%s", got)
		}
		if !strings.Contains(got, "=== Task "+failedID+" (failed) last 3 lines from _governator/_local-state/task-"+failedID+"/_governator/_local-state/worker-1-test-default/stdout.log ===\nf-1\nf-2\nf-3\n") {
			t.Fatalf("missing failed section in output:\n%s", got)
		}
		if strings.Contains(got, openID) {
			t.Fatalf("unexpected open task section in output:\n%s", got)
		}
	})

	t.Run("includes planning task section when supervisor failed during plan", func(t *testing.T) {
		indexPath := filepath.Join(tempDir, "_governator", "_local-state", "index.json")
		indexJSON := `{
  "schema_version": 1,
  "tasks": [
    {
      "id": "planning",
      "path": "_governator/planning.json",
      "kind": "planning",
      "state": "governator_planning_not_started",
      "role": "",
      "dependencies": [],
      "retries": {"max_attempts": 1},
      "attempts": {"total": 0, "failed": 0},
      "order": 0,
      "overlap": []
    }
  ]
}`
		if err := os.WriteFile(indexPath, []byte(indexJSON), 0o644); err != nil {
			t.Fatalf("write planning index: %v", err)
		}

		statePath := filepath.Join(tempDir, "_governator", "_local-state", "supervisor", "state.json")
		stateJSON := `{
  "phase": "start",
  "pid": 12345,
  "step_id": "plan",
  "step_name": "Plan",
  "state": "failed",
  "started_at": "2026-02-08T00:00:00Z",
  "last_transition": "2026-02-08T00:00:01Z",
  "log_path": "` + filepath.ToSlash(logPath) + `"
}`
		if err := os.WriteFile(statePath, []byte(stateJSON), 0o644); err != nil {
			t.Fatalf("write supervisor state: %v", err)
		}

		planningRoot := filepath.Join(tempDir, "_governator", "_local-state", "task-planning", "_governator", "_local-state")
		planningWorkerDir := filepath.Join(planningRoot, "planning-architecture-baseline")
		if err := os.MkdirAll(planningWorkerDir, 0o755); err != nil {
			t.Fatalf("mkdir planning worker dir: %v", err)
		}
		planningLog := filepath.Join(planningWorkerDir, "stdout.log")
		if err := os.WriteFile(planningLog, []byte("p-1\np-2\np-3\n"), 0o644); err != nil {
			t.Fatalf("write planning stdout log: %v", err)
		}

		cmd := exec.Command(binaryPath, "why", "-s", "1", "-t", "2")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("why with planning failure failed: %v, output: %s", err, output)
		}
		got := string(output)

		if strings.Count(got, "=== ") != 2 {
			t.Fatalf("expected exactly 2 sections (supervisor + planning), got output:\n%s", got)
		}
		if !strings.Contains(got, "=== Supervisor 12345 (failed) last 1 lines ===\nline-30\n") {
			t.Fatalf("missing supervisor section in output:\n%s", got)
		}
		if !strings.Contains(got, "=== Task planning (failed) last 2 lines from _governator/_local-state/task-planning/_governator/_local-state/planning-architecture-baseline/stdout.log ===\np-2\np-3\n") {
			t.Fatalf("missing planning section in output:\n%s", got)
		}
	})

	t.Run("falls back to stderr log when stdout log is empty", func(t *testing.T) {
		indexPath := filepath.Join(tempDir, "_governator", "_local-state", "index.json")
		indexJSON := `{
  "schema_version": 1,
  "tasks": [
    {
      "id": "T-ERR-001",
      "path": "_governator/tasks/T-ERR-001.md",
      "kind": "execution",
      "state": "triaged",
      "role": "default",
      "dependencies": [],
      "retries": {"max_attempts": 1},
      "attempts": {"total": 1, "failed": 1},
      "order": 1,
      "overlap": []
    }
  ]
}`
		if err := os.WriteFile(indexPath, []byte(indexJSON), 0o644); err != nil {
			t.Fatalf("write index: %v", err)
		}

		workerDir := filepath.Join(tempDir, "_governator", "_local-state", "task-T-ERR-001", "_governator", "_local-state", "worker-1-work-default")
		if err := os.MkdirAll(workerDir, 0o755); err != nil {
			t.Fatalf("mkdir worker dir: %v", err)
		}
		stdoutLog := filepath.Join(workerDir, "stdout.log")
		stderrLog := filepath.Join(workerDir, "stderr.log")
		if err := os.WriteFile(stdoutLog, []byte(""), 0o644); err != nil {
			t.Fatalf("write stdout log: %v", err)
		}
		if err := os.WriteFile(stderrLog, []byte("x-1\nx-2\n"), 0o644); err != nil {
			t.Fatalf("write stderr log: %v", err)
		}

		cmd := exec.Command(binaryPath, "why", "-s", "1", "-t", "2")
		cmd.Dir = tempDir
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("why with stderr fallback failed: %v, output: %s", err, output)
		}
		got := string(output)

		if !strings.Contains(got, "from _governator/_local-state/task-T-ERR-001/_governator/_local-state/worker-1-work-default/stderr.log ===\nx-1\nx-2\n") {
			t.Fatalf("missing stderr fallback section in output:\n%s", got)
		}
	})
}

func TestHandleTailQuitInput(t *testing.T) {
	t.Run("cancels on lowercase q", func(t *testing.T) {
		ctx, baseCancel := context.WithCancel(context.Background())
		defer baseCancel()
		cancelled := false
		cancel := func() {
			cancelled = true
			baseCancel()
		}
		handleTailQuitInput(ctx, strings.NewReader("abcqxyz"), io.Discard, cancel)
		if !cancelled {
			t.Fatal("expected cancel to be called for lowercase q")
		}
	})

	t.Run("cancels on uppercase q", func(t *testing.T) {
		ctx, baseCancel := context.WithCancel(context.Background())
		defer baseCancel()
		cancelled := false
		cancel := func() {
			cancelled = true
			baseCancel()
		}
		handleTailQuitInput(ctx, strings.NewReader("abQxyz"), io.Discard, cancel)
		if !cancelled {
			t.Fatal("expected cancel to be called for uppercase Q")
		}
	})

	t.Run("does not cancel without q", func(t *testing.T) {
		ctx, baseCancel := context.WithCancel(context.Background())
		defer baseCancel()
		cancelled := false
		cancel := func() {
			cancelled = true
			baseCancel()
		}
		handleTailQuitInput(ctx, strings.NewReader("abcdef"), io.Discard, cancel)
		if cancelled {
			t.Fatal("did not expect cancel to be called")
		}
	})
}
