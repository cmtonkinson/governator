// Package config provides configuration initialization helpers.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	repoDurableStateDir = "_governator/_durable_state"
	repoConfigDir       = repoDurableStateDir + "/config"
	repoLegacyConfigDir = "_governator/config"
	repoConfigFileName  = "config.json"
)

// v2DirectoryStructure defines the complete directory layout for Governator v2
var v2DirectoryStructure = []string{
	repoDurableStateDir,
	repoConfigDir,
	repoDurableStateDir + "/migrations",
	repoLegacyConfigDir,
	"_governator/docs",
	"_governator/docs/adr",
	"_governator/plan",
	"_governator/roles",
	"_governator/custom-prompts",
	"_governator/prompts",
	"_governator/_local_state",
	"_governator/_local_state/logs",
}

// InitRepoConfig creates the repository config directory and writes a minimal config file if absent.
// It does not overwrite existing configuration files.
func InitRepoConfig(repoRoot string) error {
	if repoRoot == "" {
		return fmt.Errorf("repo root cannot be empty")
	}

	legacyDir := filepath.Join(repoRoot, repoLegacyConfigDir)
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		return fmt.Errorf("create legacy config dir %s: %w", legacyDir, err)
	}

	configDir := filepath.Join(repoRoot, repoConfigDir)
	configPath := filepath.Join(configDir, repoConfigFileName)

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("create config directory %s: %w", configDir, err)
	}

	// Check if config file already exists
	if _, err := os.Stat(configPath); err == nil {
		// Config file exists, don't overwrite
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("check config file %s: %w", configPath, err)
	}

	// Write minimal default config
	defaults := Defaults()
	configData, err := json.MarshalIndent(defaults, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal default config: %w", err)
	}

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return fmt.Errorf("write config file %s: %w", configPath, err)
	}

	return nil
}

// InitFullLayout creates the complete v2 directory structure and default files.
// It is idempotent and will not overwrite existing files.
func InitFullLayout(repoRoot string) error {
	if repoRoot == "" {
		return fmt.Errorf("repo root cannot be empty")
	}

	// Create all required directories
	for _, dir := range v2DirectoryStructure {
		dirPath := filepath.Join(repoRoot, dir)
		if err := os.MkdirAll(dirPath, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dirPath, err)
		}
	}

	// Initialize config file
	if err := InitRepoConfig(repoRoot); err != nil {
		return fmt.Errorf("initialize config: %w", err)
	}

	// Create .keep files for empty directories that need to exist
	keepDirs := []string{
		"_governator/docs/adr",
		"_governator/_local_state/logs",
		repoDurableStateDir + "/migrations",
	}

	for _, dir := range keepDirs {
		keepPath := filepath.Join(repoRoot, dir, ".keep")
		if _, err := os.Stat(keepPath); os.IsNotExist(err) {
			if err := os.WriteFile(keepPath, []byte(""), 0644); err != nil {
				return fmt.Errorf("create .keep file %s: %w", keepPath, err)
			}
		}
	}

	return nil
}
