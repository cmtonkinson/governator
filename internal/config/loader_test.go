// Tests for configuration loading.
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadConfigPrecedence verifies precedence across user, repo, and CLI layers.
func TestLoadConfigPrecedence(t *testing.T) {
	homeDir := t.TempDir()
	repoRoot := filepath.Join(t.TempDir(), "repo")
	t.Setenv("HOME", homeDir)

	userConfigDir := filepath.Join(homeDir, userConfigDirName, "governator")
	repoConfigDir := filepath.Join(repoRoot, repoConfigDirName)

	writeConfigFile(t, filepath.Join(userConfigDir, userConfigFileName), `{
  "concurrency": {
    "global": 2
  },
  "auto_rerun": {
    "enabled": true
  },
  "timeouts": {
    "worker_seconds": 300
  },
  "workers": {
    "commands": {
      "default": ["user", "{task_path}"]
    }
  }
}`)

	writeConfigFile(t, filepath.Join(repoConfigDir, userConfigFileName), `{
  "concurrency": {
    "global": 3
  },
  "timeouts": {
    "worker_seconds": 120
  }
}`)

	cliOverrides := map[string]any{
		"concurrency": map[string]any{
			"global": 4,
		},
		"auto_rerun": map[string]any{
			"enabled": false,
		},
	}

	cfg, err := Load(repoRoot, cliOverrides, nil)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}

	if cfg.Concurrency.Global != 4 {
		t.Fatalf("concurrency.global = %d, want 4", cfg.Concurrency.Global)
	}
	if cfg.Timeouts.WorkerSeconds != 120 {
		t.Fatalf("timeouts.worker_seconds = %d, want 120", cfg.Timeouts.WorkerSeconds)
	}
	if cfg.AutoRerun.Enabled {
		t.Fatal("auto_rerun.enabled should be false after CLI override")
	}
	if len(cfg.Workers.Commands.Default) != 2 || cfg.Workers.Commands.Default[0] != "user" {
		t.Fatalf("workers.commands.default should come from user defaults, got %v", cfg.Workers.Commands.Default)
	}
}

// TestLoadConfigInvalidJSON verifies invalid JSON yields a clear error.
func TestLoadConfigInvalidJSON(t *testing.T) {
	homeDir := t.TempDir()
	t.Setenv("HOME", homeDir)

	userConfigDir := filepath.Join(homeDir, userConfigDirName, "governator")
	writeConfigFile(t, filepath.Join(userConfigDir, userConfigFileName), `{"workers":`)

	_, err := Load("", nil, nil)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
	if !strings.Contains(err.Error(), "user defaults") {
		t.Fatalf("expected error to mention user defaults, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), userConfigFileName) {
		t.Fatalf("expected error to mention config.json, got %q", err.Error())
	}
}

// writeConfigFile creates a config file with the provided contents.
func writeConfigFile(t *testing.T, path string, contents string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
