package config

import (
	"bytes"
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
		configDir := filepath.Join(tempDir, repoDurableStateDir)
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

	t.Run("seeds planning prompts from embedded templates", func(t *testing.T) {
		tempDir := t.TempDir()

		if err := InitFullLayout(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitFullLayout failed: %v", err)
		}

		for _, prompt := range planningPromptTemplates {
			path := filepath.Join(tempDir, "_governator", "prompts", prompt.name)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read planning prompt %s: %v", prompt.name, err)
			}
			expected, err := templates.Read(prompt.template)
			if err != nil {
				t.Fatalf("read embedded template %s: %v", prompt.template, err)
			}
			if !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(expected)) {
				t.Fatalf("planning prompt %s mismatch", prompt.name)
			}
		}
	})

	t.Run("seeds planning spec from embedded template", func(t *testing.T) {
		tempDir := t.TempDir()

		if err := InitFullLayout(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitFullLayout failed: %v", err)
		}

		path := filepath.Join(tempDir, "_governator", "planning.json")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read planning spec: %v", err)
		}
		expected, err := templates.Read("planning/planning.json")
		if err != nil {
			t.Fatalf("read embedded planning spec: %v", err)
		}
		if !bytes.Equal(bytes.TrimSpace(data), bytes.TrimSpace(expected)) {
			t.Fatalf("planning spec mismatch")
		}
	})

	t.Run("preserves existing config file", func(t *testing.T) {
		// Create temporary directory for test
		tempDir := t.TempDir()

		// Create config directory and file with custom content
		configDir := filepath.Join(tempDir, repoDurableStateDir)
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
		governatorDir := filepath.Join(tempDir, "_governator")
		if _, err := os.Stat(governatorDir); os.IsNotExist(err) {
			t.Errorf("_governator directory was not created")
		}

		durableStateDir := filepath.Join(tempDir, repoDurableStateDir)
		if _, err := os.Stat(durableStateDir); os.IsNotExist(err) {
			t.Errorf("durable state directory %s was not created", repoDurableStateDir)
		}

		legacyConfigDir := filepath.Join(tempDir, "_governator", "config")
		if _, err := os.Stat(legacyConfigDir); err == nil {
			t.Errorf("legacy config dir %s should not be created", legacyConfigDir)
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

		// Verify reasoning prompts were created
		reasoningFiles := []string{
			"_governator/reasoning/high.md",
			"_governator/reasoning/medium.md",
			"_governator/reasoning/low.md",
		}
		for _, file := range reasoningFiles {
			filePath := filepath.Join(tempDir, file)
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				t.Errorf("Reasoning prompt %s was not created", file)
			}
		}

		// Verify config file was created
		configPath := filepath.Join(tempDir, repoDurableStateDir, repoConfigFileName)
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			t.Errorf("Config file was not created")
		}

		// Verify .keep files were created
		for _, dir := range v2DirectoryStructure {
			keepPath := filepath.Join(tempDir, dir, ".keep")
			if _, err := os.Stat(keepPath); os.IsNotExist(err) {
				t.Errorf(".keep file for %s was not created", dir)
			}
		}
	})

	t.Run("copies embedded templates into _governator/templates", func(t *testing.T) {
		tempDir := t.TempDir()

		if err := InitFullLayout(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitFullLayout failed: %v", err)
		}

		for _, name := range templates.Required() {
			templatePath := filepath.Join(tempDir, templatesDirName, templates.LocalFilename(name))
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
		configDir := filepath.Join(tempDir, repoDurableStateDir)
		if err := os.MkdirAll(configDir, 0755); err != nil {
			t.Fatalf("Failed to create config dir: %v", err)
		}

		configPath := filepath.Join(configDir, "config.json")
		customConfig := `{"concurrency": {"global": 10}}`
		if err := os.WriteFile(configPath, []byte(customConfig), 0644); err != nil {
			t.Fatalf("Failed to write custom config: %v", err)
		}

		keepPath := filepath.Join(tempDir, "_governator", "architecture", ".keep")
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

func TestApplyInitOverrides(t *testing.T) {
	t.Run("applies all overrides correctly", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create initial config
		if err := InitRepoConfig(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitRepoConfig failed: %v", err)
		}

		// Apply overrides
		overrides := InitOverrides{
			Agent:           "claude",
			Concurrency:     5,
			ReasoningEffort: "high",
			Branch:          "develop",
			Timeout:         1800,
		}

		if err := ApplyInitOverrides(tempDir, overrides); err != nil {
			t.Fatalf("ApplyInitOverrides failed: %v", err)
		}

		// Read config and verify
		configPath := filepath.Join(tempDir, repoDurableStateDir, repoConfigFileName)
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config: %v", err)
		}

		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("Failed to parse config: %v", err)
		}

		if cfg.Workers.CLI.Default != "claude" {
			t.Errorf("Agent override failed: got %s, want claude", cfg.Workers.CLI.Default)
		}
		if cfg.Concurrency.Global != 5 {
			t.Errorf("Concurrency.Global override failed: got %d, want 5", cfg.Concurrency.Global)
		}
		if cfg.Concurrency.DefaultRole != 5 {
			t.Errorf("Concurrency.DefaultRole override failed: got %d, want 5", cfg.Concurrency.DefaultRole)
		}
		if cfg.ReasoningEffort.Default != "high" {
			t.Errorf("ReasoningEffort override failed: got %s, want high", cfg.ReasoningEffort.Default)
		}
		if cfg.Branches.Base != "develop" {
			t.Errorf("Branch override failed: got %s, want develop", cfg.Branches.Base)
		}
		if cfg.Timeouts.WorkerSeconds != 1800 {
			t.Errorf("Timeout override failed: got %d, want 1800", cfg.Timeouts.WorkerSeconds)
		}
	})

	t.Run("applies partial overrides", func(t *testing.T) {
		tempDir := t.TempDir()

		if err := InitRepoConfig(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitRepoConfig failed: %v", err)
		}

		// Only override agent
		overrides := InitOverrides{
			Agent: "gemini",
		}

		if err := ApplyInitOverrides(tempDir, overrides); err != nil {
			t.Fatalf("ApplyInitOverrides failed: %v", err)
		}

		configPath := filepath.Join(tempDir, repoDurableStateDir, repoConfigFileName)
		data, err := os.ReadFile(configPath)
		if err != nil {
			t.Fatalf("Failed to read config: %v", err)
		}

		var cfg Config
		if err := json.Unmarshal(data, &cfg); err != nil {
			t.Fatalf("Failed to parse config: %v", err)
		}

		if cfg.Workers.CLI.Default != "gemini" {
			t.Errorf("Agent override failed: got %s, want gemini", cfg.Workers.CLI.Default)
		}
		// Other values should remain at defaults
		if cfg.Concurrency.Global != defaultConcurrencyGlobal {
			t.Errorf("Concurrency.Global should remain default: got %d, want %d", cfg.Concurrency.Global, defaultConcurrencyGlobal)
		}
	})

	t.Run("rejects invalid agent", func(t *testing.T) {
		tempDir := t.TempDir()

		if err := InitRepoConfig(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitRepoConfig failed: %v", err)
		}

		overrides := InitOverrides{
			Agent: "invalid-cli",
		}

		err := ApplyInitOverrides(tempDir, overrides)
		if err == nil {
			t.Fatal("Expected error for invalid agent, got nil")
		}
		if !bytes.Contains([]byte(err.Error()), []byte("invalid agent")) {
			t.Errorf("Expected 'invalid agent' error, got: %v", err)
		}
	})

	t.Run("rejects invalid reasoning effort", func(t *testing.T) {
		tempDir := t.TempDir()

		if err := InitRepoConfig(tempDir, InitOptions{}); err != nil {
			t.Fatalf("InitRepoConfig failed: %v", err)
		}

		overrides := InitOverrides{
			ReasoningEffort: "invalid-effort",
		}

		err := ApplyInitOverrides(tempDir, overrides)
		if err == nil {
			t.Fatal("Expected error for invalid reasoning effort, got nil")
		}
		if !bytes.Contains([]byte(err.Error()), []byte("invalid reasoning-effort")) {
			t.Errorf("Expected 'invalid reasoning-effort' error, got: %v", err)
		}
	})
}
