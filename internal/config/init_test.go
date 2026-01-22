package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/templates"
)

func TestInitRepoConfig(t *testing.T) {
	t.Run("creates config directory and file in clean repo", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		// Run init
		err := InitRepoConfig(tempDir, InitOptions{})
		if err != nil {
			t.Fatalf("InitRepoConfig failed: %v", err)
		}

		// Verify config directory exists
		configDir := filepath.Join(tempDir, repoConfigDir)
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			t.Errorf("Config directory %s was not created", configDir)
		}

		// Verify config file exists
		configPath := filepath.Join(configDir, repoConfigFileName)
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Errorf("Config file %s was not created", configPath)
		}

		// Verify config file contains valid JSON with defaults
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("Config file contains invalid JSON: %v", err)
		}

		// Verify it matches defaults
		expected := Defaults()
		if len(cfg.Workers.Commands.Default) != len(expected.Workers.Commands.Default) {
			t.Errorf("Default command mismatch: got %v, want %v", cfg.Workers.Commands.Default, expected.Workers.Commands.Default)
		}
		if cfg.Concurrency.Global != expected.Concurrency.Global {
			t.Errorf("Global concurrency mismatch: got %d, want %d", cfg.Concurrency.Global, expected.Concurrency.Global)
		}
	})

	t.Run("preserves existing config file", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		// Create config directory and file with custom content
		configDir := filepath.Join(tempDir, repoConfigDir)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("Failed to create config dir: %v", err)
		}

		configPath := filepath.Join(configDir, repoConfigFileName)
		customConfig := `{"concurrency": {"global": 5}}`
		if err := os.WriteFile(configPath, []byte(customConfig), 0644); err != nil {
			t.Fatalf("Failed to write custom config: %v", err)
		}

		// Run init
		err := InitRepoConfig(tempDir, InitOptions{})
		if err != nil {
			t.Fatalf("InitRepoConfig failed: %v", err)
		}

		// Verify existing config is preserved
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		if string(data) != customConfig {
			t.Errorf("Existing config was overwritten: got %s, want %s", string(data), customConfig)
		}
	})

	t.Run("handles empty repo root", func(t *testing.T) {
		err := InitRepoConfig("", InitOptions{})
		if err == nil {
			t.Error("Expected error for empty repo root, got nil")
		}
		if err.Error() != "repo root cannot be empty" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})

	t.Run("creates nested directories", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		// Run init
		err := InitRepoConfig(tempDir, InitOptions{})
		if err != nil {
			t.Fatalf("InitRepoConfig failed: %v", err)
		}

		// Verify nested directory structure
		configDir := filepath.Join(tempDir, "_governator")
		if _, err := os.Stat(configDir); os.IsNotExist(err) {
			t.Errorf("_governator directory was not created")
		}

		configSubDir := filepath.Join(tempDir, "_governator", "config")
		if _, err := os.Stat(configSubDir); os.IsNotExist(err) {
			t.Errorf("_governator/config directory was not created")
		}
	})
}

func TestInitFullLayout(t *testing.T) {
	t.Run("creates complete directory structure in clean repo", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		// Run full layout init
		err := InitFullLayout(tempDir, InitOptions{})
		if err != nil {
			t.Fatalf("InitFullLayout failed: %v", err)
		}

		// Verify all required directories exist
		for _, dir := range v2DirectoryStructure {
			dirPath := filepath.Join(tempDir, dir)
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				t.Errorf("Directory %s was not created", dir)
			}
		}

		// Verify config file was created
		configPath := filepath.Join(tempDir, repoConfigDir, repoConfigFileName)
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Errorf("Config file was not created")
		}

		// Verify .keep files were created
		keepFiles := []string{
			"_governator/docs/adr/.keep",
			"_governator/_local_state/logs/.keep",
			"_governator/_durable_state/migrations/.keep",
		}

		for _, keepFile := range keepFiles {
			keepPath := filepath.Join(tempDir, keepFile)
			if _, err := os.Stat(keepPath); os.IsNotExist(err) {
				t.Errorf(".keep file %s was not created", keepFile)
			}
		}
	})

	t.Run("copies embedded templates into _governator/templates", func(t *testing.T) {
		tempDir := t.TempDir()

		if err := InitFullLayout(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitFullLayout failed: %v", err)
		}

		for _, name := range templates.Required() {
			templatePath := filepath.Join(tempDir, templatesDirName, filepath.FromSlash(name))
			if _, err := os.Stat(templatePath); os.IsNotExist(err) {
				t.Errorf("template %s was not copied", name)
			}
		}
	})

	t.Run("is idempotent - does not fail on existing directories", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		// Run init twice
		err := InitFullLayout(tempDir, InitOptions{})
		if err != nil {
			t.Fatalf("First InitFullLayout failed: %v", err)
		}

		err = InitFullLayout(tempDir, InitOptions{})
		if err != nil {
			t.Fatalf("Second InitFullLayout failed: %v", err)
		}

		// Verify directories still exist
		for _, dir := range v2DirectoryStructure {
			dirPath := filepath.Join(tempDir, dir)
			if _, err := os.Stat(dirPath); os.IsNotExist(err) {
				t.Errorf("Directory %s was not preserved after second init", dir)
			}
		}
	})

	t.Run("preserves existing files", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		// Create some existing files
		configDir := filepath.Join(tempDir, repoConfigDir)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("Failed to create config dir: %v", err)
		}

		configPath := filepath.Join(configDir, "config.json")
		customConfig := `{"concurrency": {"global": 10}}`
		if err := os.WriteFile(configPath, []byte(customConfig), 0644); err != nil {
			t.Fatalf("Failed to write custom config: %v", err)
		}

		keepPath := filepath.Join(tempDir, "_governator", "docs", "adr", ".keep")
		if err := os.MkdirAll(filepath.Dir(keepPath), 0755); err != nil {
			t.Fatalf("Failed to create adr dir: %v", err)
		}
		if err := os.WriteFile(keepPath, []byte("existing"), 0644); err != nil {
			t.Fatalf("Failed to write existing .keep: %v", err)
		}

		// Run init
		err := InitFullLayout(tempDir, InitOptions{})
		if err != nil {
			t.Fatalf("InitFullLayout failed: %v", err)
		}

		// Verify existing config is preserved
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config file: %v", err)
		}

		if string(data) != customConfig {
			t.Errorf("Existing config was overwritten: got %s, want %s", string(data), customConfig)
		}

		// Verify existing .keep file is preserved
		keepData, err := os.ReadFile(keepPath)
		if err != nil {
			t.Fatalf("Failed to read .keep file: %v", err)
		}

		if string(keepData) != "existing" {
			t.Errorf("Existing .keep file was overwritten: got %s, want %s", string(keepData), "existing")
		}
	})

	t.Run("handles empty repo root", func(t *testing.T) {
		err := InitFullLayout("", InitOptions{})
		if err == nil {
			t.Error("Expected error for empty repo root, got nil")
		}
		if err.Error() != "repo root cannot be empty" {
			t.Errorf("Unexpected error message: %v", err)
		}
	})
}
