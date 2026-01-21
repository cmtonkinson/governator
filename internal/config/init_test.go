package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInitRepoConfig(t *testing.T) {
	t.Run("creates config directory and file in clean repo", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()
		
		// Run init
		err := InitRepoConfig(tempDir)
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
		err := InitRepoConfig(tempDir)
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
		err := InitRepoConfig("")
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
		err := InitRepoConfig(tempDir)
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