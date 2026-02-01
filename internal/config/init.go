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
	repoDurableStateDir = "_governator/_durable-state"
	repoConfigFileName  = "config.json"
	templatesDirName    = "_governator/templates"
)

// v2DirectoryStructure defines the complete directory layout for Governator v2.
// Each entry is created by gov init and should include a .keep file so Git persists the tree.
var v2DirectoryStructure = []string{
	"_governator",
	repoDurableStateDir,
	filepath.Join(repoDurableStateDir, "migrations"),
	"_governator/docs",
	filepath.Join("_governator", "docs", "adr"),
	"_governator/task",
	"_governator/roles",
	"_governator/custom-prompts",
	"_governator/prompts",
	"_governator/templates",
	"_governator/reasoning",
	filepath.Join("_governator", "_local-state"),
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

	configDir := filepath.Join(repoRoot, repoDurableStateDir)
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
		keepPath := filepath.Join(dirPath, ".keep")
		if err := ensureKeepFile(keepPath, opts); err != nil {
			return fmt.Errorf("create .keep file %s: %w", keepPath, err)
		}
	}

	// Initialize config file
	if err := InitRepoConfig(repoRoot, opts); err != nil {
		return fmt.Errorf("initialize config: %w", err)
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

	if err := ensurePlanningSpec(repoRoot, opts); err != nil {
		return fmt.Errorf("create planning spec: %w", err)
	}

	if err := ensureWorkerContract(repoRoot, opts); err != nil {
		return fmt.Errorf("create worker contract: %w", err)
	}

	if err := ensureReasoningPrompts(repoRoot, opts); err != nil {
		return fmt.Errorf("create reasoning prompts: %w", err)
	}

	if err := ensureCustomPrompts(repoRoot, opts); err != nil {
		return fmt.Errorf("create custom prompts: %w", err)
	}

	if err := ensureGitignore(repoRoot, opts); err != nil {
		return fmt.Errorf("create gitignore: %w", err)
	}

	return nil
}

func ensureRolePrompts(repoRoot string, opts InitOptions) error {
	roles := []string{"architect", "default", "planner"}
	rolesDir := filepath.Join(repoRoot, "_governator", "roles")
	if err := ensureDir(rolesDir, opts); err != nil {
		return fmt.Errorf("ensure roles directory %s: %w", rolesDir, err)
	}
	for _, role := range roles {
		path := filepath.Join(rolesDir, fmt.Sprintf("%s.md", role))
		if exists, err := pathExists(path); err != nil {
			return fmt.Errorf("check role prompt %s: %w", path, err)
		} else if exists {
			continue
		}
		template := fmt.Sprintf("roles/%s.md", role)
		data, err := templates.Read(template)
		if err != nil {
			return fmt.Errorf("read role template %s: %w", template, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write role prompt %s: %w", path, err)
		}
		opts.logf("created role prompt %s", repoRelativePath(repoRoot, path))
	}
	return nil
}

var planningPromptTemplates = []struct {
	name     string
	template string
}{
	{name: "architecture-baseline.md", template: "planning/architecture-baseline.md"},
	{name: "gap-analysis.md", template: "planning/gap-analysis.md"},
	{name: "roadmap.md", template: "planning/roadmap.md"},
	{name: "task-planning.md", template: "planning/plan-tasks.md"},
}

func ensurePlanningPrompts(repoRoot string, opts InitOptions) error {
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := ensureDir(promptsDir, opts); err != nil {
		return fmt.Errorf("create planning prompts directory %s: %w", promptsDir, err)
	}
	for _, prompt := range planningPromptTemplates {
		path := filepath.Join(promptsDir, prompt.name)
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("stat planning prompt %s: %w", path, err)
		}
		data, err := templates.Read(prompt.template)
		if err != nil {
			return fmt.Errorf("read planning template %s: %w", prompt.template, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write planning prompt %s: %w", path, err)
		}
		opts.logf("created planning prompt %s", repoRelativePath(repoRoot, path))
	}
	return nil
}

// ensurePlanningSpec writes the planning workstream spec if it does not already exist.
func ensurePlanningSpec(repoRoot string, opts InitOptions) error {
	path := filepath.Join(repoRoot, "_governator", "planning.json")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat planning spec %s: %w", path, err)
	}
	data, err := templates.Read("planning/planning.json")
	if err != nil {
		return fmt.Errorf("read planning spec template: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write planning spec %s: %w", path, err)
	}
	opts.logf("created planning spec %s", repoRelativePath(repoRoot, path))
	return nil
}

func ensureWorkerContract(repoRoot string, opts InitOptions) error {
	path := filepath.Join(repoRoot, "_governator", "worker-contract.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat worker contract %s: %w", path, err)
	}
	data, err := templates.Read("worker-contract.md")
	if err != nil {
		return fmt.Errorf("read worker contract template: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write worker contract %s: %w", path, err)
	}
	opts.logf("created worker contract %s", repoRelativePath(repoRoot, path))
	return nil
}

func ensureCustomPrompts(repoRoot string, opts InitOptions) error {
	promptsDir := filepath.Join(repoRoot, "_governator", "custom-prompts")
	if err := ensureDir(promptsDir, opts); err != nil {
		return fmt.Errorf("create custom prompts directory %s: %w", promptsDir, err)
	}
	path := filepath.Join(promptsDir, "_global.md")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat custom prompt %s: %w", path, err)
	}
	data, err := templates.Read("custom-prompts/_global.md")
	if err != nil {
		return fmt.Errorf("read custom prompt template custom-prompts/_global.md: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write custom prompt %s: %w", path, err)
	}
	opts.logf("created custom prompt %s", repoRelativePath(repoRoot, path))

	// Seed role-specific custom prompt placeholders.
	roles := []string{"architect", "default", "planner"}
	for _, role := range roles {
		rolePath := filepath.Join(promptsDir, fmt.Sprintf("%s.md", role))
		if exists, err := pathExists(rolePath); err != nil {
			return fmt.Errorf("check custom prompt %s: %w", rolePath, err)
		} else if exists {
			continue
		}
		template := fmt.Sprintf("custom-prompts/%s.md", role)
		data, err := templates.Read(template)
		if err != nil {
			return fmt.Errorf("read custom prompt template %s: %w", template, err)
		}
		if err := os.WriteFile(rolePath, data, 0o644); err != nil {
			return fmt.Errorf("write custom prompt %s: %w", rolePath, err)
		}
		opts.logf("created custom prompt %s", repoRelativePath(repoRoot, rolePath))
	}
	return nil
}

func ensureGitignore(repoRoot string, opts InitOptions) error {
	gitignoreDir := filepath.Join(repoRoot, "_governator")
	if err := ensureDir(gitignoreDir, opts); err != nil {
		return fmt.Errorf("create governator dir %s: %w", gitignoreDir, err)
	}
	path := filepath.Join(gitignoreDir, ".gitignore")
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat gitignore %s: %w", path, err)
	}
	content := "_local-state/*\n!_local-state/.keep\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write gitignore %s: %w", path, err)
	}
	opts.logf("created file %s", repoRelativePath(repoRoot, path))
	return nil
}

func ensureReasoningPrompts(repoRoot string, opts InitOptions) error {
	reasoningDir := filepath.Join(repoRoot, "_governator", "reasoning")
	if err := ensureDir(reasoningDir, opts); err != nil {
		return fmt.Errorf("create reasoning directory %s: %w", reasoningDir, err)
	}
	prompts := []string{"high.md", "medium.md", "low.md"}
	for _, name := range prompts {
		path := filepath.Join(reasoningDir, name)
		exists, err := pathExists(path)
		if err != nil {
			return fmt.Errorf("check reasoning prompt %s: %w", path, err)
		}
		if exists {
			continue
		}
		data, err := templates.Read(filepath.ToSlash(filepath.Join("reasoning", name)))
		if err != nil {
			return fmt.Errorf("read reasoning template %s: %w", name, err)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write reasoning prompt %s: %w", path, err)
		}
		opts.logf("created reasoning prompt %s", repoRelativePath(repoRoot, path))
	}
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
		localPath := filepath.Join(templatesDir, templates.LocalFilename(name))
		exists, err := pathExists(localPath)
		if err != nil {
			return fmt.Errorf("check template %s: %w", name, err)
		}
		if exists {
			continue
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
