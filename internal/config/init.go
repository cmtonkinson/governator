// Package config provides configuration initialization helpers.
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	repoConfigDir      = "_governator/config"
	repoConfigFileName = "config.json"
)

// InitRepoConfig creates the repository config directory and writes a minimal config file if absent.
// It does not overwrite existing configuration files.
func InitRepoConfig(repoRoot string) error {
	if repoRoot == "" {
		return fmt.Errorf("repo root cannot be empty")
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