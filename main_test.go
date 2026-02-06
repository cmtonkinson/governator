package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
