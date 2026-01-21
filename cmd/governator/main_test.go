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