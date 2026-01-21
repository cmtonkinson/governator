package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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
			name:           "no arguments shows usage",
			args:           []string{},
			expectedExit:   2,
			expectedError:  "usage: governator <init|plan|run|status|version>",
		},
		{
			name:           "unknown command shows usage",
			args:           []string{"unknown"},
			expectedExit:   2,
			expectedError:  "usage: governator <init|plan|run|status|version>",
		},
		{
			name:           "version command",
			args:           []string{"version"},
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

	output, err := exec.Command(binaryPath, "version").CombinedOutput()
	if err != nil {
		t.Fatalf("Version command failed: %v, output: %s", err, output)
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
		"_governator/config",
		"_governator/docs",
		"_governator/docs/adr",
		"_governator/plan",
		"_governator/roles",
		"_governator/custom-prompts",
		"_governator/prompts",
		"_governator/_local_state",
		"_governator/_local_state/logs",
	}
	
	for _, dir := range expectedDirs {
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			t.Errorf("Expected directory %s was not created", dir)
		}
	}
	
	// Check that config file was created
	configPath := filepath.Join("_governator", "config", "config.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("Expected config.json was not created")
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
	if _, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("Init command failed: %v", err)
	}

	t.Run("status with empty index", func(t *testing.T) {
		// Create an empty task index
		planDir := filepath.Join(tempDir, "_governator", "plan")
		if err := os.MkdirAll(planDir, 0755); err != nil {
			t.Fatalf("Failed to create plan dir: %v", err)
		}

		// Write empty index file
		indexPath := filepath.Join(planDir, "task-index.json")
		emptyIndex := `{"schema_version":1,"tasks":[]}`
		if err := os.WriteFile(indexPath, []byte(emptyIndex), 0644); err != nil {
			t.Fatalf("Failed to write empty index: %v", err)
		}

		cmd := exec.Command(binaryPath, "status")
		output, err := cmd.CombinedOutput()
		
		if err != nil {
			t.Fatalf("Status command failed: %v, output: %s", err, output)
		}
		
		outputStr := strings.TrimSpace(string(output))
		expected := "tasks total=0 done=0 open=0 blocked=0"
		if outputStr != expected {
			t.Errorf("Expected %q, got %q", expected, outputStr)
		}
	})

	t.Run("status with populated index", func(t *testing.T) {
		// Create a populated task index
		indexPath := filepath.Join(tempDir, "_governator", "plan", "task-index.json")
		populatedIndex := `{
			"schema_version": 1,
			"tasks": [
				{"id": "T-001", "state": "done"},
				{"id": "T-002", "state": "done"},
				{"id": "T-003", "state": "open"},
				{"id": "T-004", "state": "open"},
				{"id": "T-005", "state": "blocked"},
				{"id": "T-006", "state": "worked"},
				{"id": "T-007", "state": "conflict"}
			]
		}`
		if err := os.WriteFile(indexPath, []byte(populatedIndex), 0644); err != nil {
			t.Fatalf("Failed to write populated index: %v", err)
		}

		cmd := exec.Command(binaryPath, "status")
		output, err := cmd.CombinedOutput()
		
		if err != nil {
			t.Fatalf("Status command failed: %v, output: %s", err, output)
		}
		
		outputStr := strings.TrimSpace(string(output))
		expected := "tasks total=7 done=2 open=3 blocked=2"
		if outputStr != expected {
			t.Errorf("Expected %q, got %q", expected, outputStr)
		}
	})
}
