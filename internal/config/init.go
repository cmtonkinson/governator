// Package config provides configuration initialization helpers.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/cmtonkinson/governator/internal/templates"
)

const (
	repoDurableStateDir = "_governator/_durable_state"
	repoConfigDir       = repoDurableStateDir + "/config"
	repoLegacyConfigDir = "_governator/config"
	repoConfigFileName  = "config.json"
	templatesDirName    = "_governator/templates"
)

// v2DirectoryStructure defines the complete directory layout for Governator v2
var v2DirectoryStructure = []string{
	repoDurableStateDir,
	repoConfigDir,
	repoDurableStateDir + "/migrations",
	repoLegacyConfigDir,
	"_governator/docs",
	"_governator/plan",
	"_governator/docs/adr",
	"_governator/roles",
	"_governator/custom-prompts",
	"_governator/prompts",
	"_governator/_local_state",
	"_governator/_local_state/logs",
	templatesDirName,
}

// InitOptions configures init-time behaviors such as verbose logging.
type InitOptions struct {
	Verbose bool
	Writer  io.Writer
}

func (opts InitOptions) logf(format string, args ...interface{}) {
	if !opts.Verbose {
		return
	}
	writer := opts.Writer
	if writer == nil {
		writer = os.Stdout
	}
	fmt.Fprintf(writer, format+"\n", args...)
}

// InitRepoConfig creates the repository config directory and writes a minimal config file if absent.
// It does not overwrite existing configuration files.
func InitRepoConfig(repoRoot string, opts InitOptions) error {
	if repoRoot == "" {
		return fmt.Errorf("repo root cannot be empty")
	}

	legacyDir := filepath.Join(repoRoot, repoLegacyConfigDir)
	if err := ensureDir(legacyDir, opts); err != nil {
		return fmt.Errorf("create legacy config dir %s: %w", legacyDir, err)
	}

	configDir := filepath.Join(repoRoot, repoConfigDir)
	configPath := filepath.Join(configDir, repoConfigFileName)

	// Create config directory if it doesn't exist
	if err := ensureDir(configDir, opts); err != nil {
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
	opts.logf("created file %s", repoRelativePath(repoRoot, configPath))

	return nil
}

// InitFullLayout creates the complete v2 directory structure and default files.
// It is idempotent and will not overwrite existing files.
func InitFullLayout(repoRoot string, opts InitOptions) error {
	if repoRoot == "" {
		return fmt.Errorf("repo root cannot be empty")
	}

	// Create all required directories
	for _, dir := range v2DirectoryStructure {
		dirPath := filepath.Join(repoRoot, dir)
		if err := ensureDir(dirPath, opts); err != nil {
			return fmt.Errorf("create directory %s: %w", dirPath, err)
		}
	}

	// Initialize config file
	if err := InitRepoConfig(repoRoot, opts); err != nil {
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
		if err := ensureKeepFile(keepPath, opts); err != nil {
			return fmt.Errorf("create .keep file %s: %w", keepPath, err)
		}
	}

	if err := ensureTemplates(repoRoot, opts); err != nil {
		return fmt.Errorf("create templates: %w", err)
	}

	if err := ensureRolePrompts(repoRoot, opts); err != nil {
		return fmt.Errorf("create role prompts: %w", err)
	}

	if err := ensurePlanningPrompts(repoRoot, opts); err != nil {
		return fmt.Errorf("create planning prompts: %w", err)
	}

	if err := ensureWorkerContract(repoRoot, opts); err != nil {
		return fmt.Errorf("create worker contract: %w", err)
	}

	return nil
}

func ensureRolePrompts(repoRoot string, opts InitOptions) error {
	roles := []string{"architect", "generalist", "planner"}
	rolesDir := filepath.Join(repoRoot, "_governator", "roles")
	for _, role := range roles {
		path := filepath.Join(rolesDir, role+".md")
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat role prompt %s: %w", path, err)
		}
		content := fmt.Sprintf("# Role: %s\n\nGuidance for the %s agent role.\n", role, role)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("write role prompt %s: %w", path, err)
		}
		opts.logf("created role prompt %s", repoRelativePath(repoRoot, path))
	}
	return nil
}

func ensurePlanningPrompts(repoRoot string, opts InitOptions) error {
	prompts := map[string]string{
		"architecture-baseline.md": "Architecture baseline planning prompt placeholder.",
		"gap-analysis.md":          "Gap analysis planning prompt placeholder.",
		"roadmap.md":               "Roadmap planning prompt placeholder.",
		"task-planning.md":         "Task planning prompt placeholder.",
	}
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	for name, content := range prompts {
		path := filepath.Join(promptsDir, name)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat planning prompt %s: %w", path, err)
		}
		if err := os.WriteFile(path, []byte(content+"\n"), 0o644); err != nil {
			return fmt.Errorf("write planning prompt %s: %w", path, err)
		}
		opts.logf("created planning prompt %s", repoRelativePath(repoRoot, path))
	}
	return nil
}

func ensureWorkerContract(repoRoot string, opts InitOptions) error {
	path := filepath.Join(repoRoot, "_governator", "worker-contract.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat worker contract %s: %w", path, err)
	}
	content := "# Worker Contract\n\nPlease obey the worker contract.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write worker contract %s: %w", path, err)
	}
	opts.logf("created worker contract %s", repoRelativePath(repoRoot, path))
	return nil
}

func ensureDir(path string, opts InitOptions) error {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("path %s exists but is not a directory", path)
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(path, 0755); err != nil {
		return err
	}
	opts.logf("created directory %s", path)
	return nil
}

func ensureKeepFile(path string, opts InitOptions) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.WriteFile(path, []byte(""), 0644); err != nil {
		return err
	}
	opts.logf("created file %s", path)
	return nil
}

func ensureTemplates(repoRoot string, opts InitOptions) error {
	templatesDir := filepath.Join(repoRoot, templatesDirName)
	if err := ensureDir(templatesDir, opts); err != nil {
		return err
	}

	for _, name := range templates.Required() {
		localPath := filepath.Join(templatesDir, filepath.FromSlash(name))
		exists, err := pathExists(localPath)
		if err != nil {
			return fmt.Errorf("check template %s: %w", name, err)
		}
		if exists {
			continue
		}

		dir := filepath.Dir(localPath)
		if err := ensureDir(dir, opts); err != nil {
			return fmt.Errorf("create template directory %s: %w", dir, err)
		}

		data, err := templates.Read(name)
		if err != nil {
			return fmt.Errorf("read embedded template %s: %w", name, err)
		}
		if err := os.WriteFile(localPath, data, 0644); err != nil {
			return fmt.Errorf("write template %s: %w", localPath, err)
		}
		opts.logf("created template %s", repoRelativePath(repoRoot, localPath))
	}

	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func repoRelativePath(repoRoot, target string) string {
	rel, err := filepath.Rel(repoRoot, target)
	if err != nil {
		return filepath.ToSlash(target)
	}
	return filepath.ToSlash(rel)
}
