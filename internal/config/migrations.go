// Package config provides repository migration helpers for durable layout updates.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/cmtonkinson/governator/internal/templates"
)

const (
	conflictResolutionPromptName       = "conflict-resolution.md"
	conflictResolutionTemplatePath     = "planning/conflict-resolution.md"
	conflictResolutionMigrationID      = "20260209_add_conflict_resolution_prompt"
	conflictResolutionNormalizedPrompt = "conflictresolution"
)

type repoMigration struct {
	id    string
	apply func(repoRoot string, opts InitOptions) error
}

var repoMigrations = []repoMigration{
	{
		id:    conflictResolutionMigrationID,
		apply: migrateConflictResolutionPrompt,
	},
}

// PendingRepoMigrations returns repo migration IDs that have not yet been marked complete.
func PendingRepoMigrations(repoRoot string) ([]string, error) {
	if strings.TrimSpace(repoRoot) == "" {
		return nil, fmt.Errorf("repo root cannot be empty")
	}

	migrationsDir := filepath.Join(repoRoot, repoDurableStateDir, "migrations")
	pending := make([]string, 0, len(repoMigrations))
	for _, migration := range repoMigrations {
		if strings.TrimSpace(migration.id) == "" {
			continue
		}
		markerPath := filepath.Join(migrationsDir, migration.id+".done")
		exists, err := pathExists(markerPath)
		if err != nil {
			return nil, fmt.Errorf("check migration marker %s: %w", markerPath, err)
		}
		if !exists {
			pending = append(pending, migration.id)
		}
	}
	return pending, nil
}

// ApplyRepoMigrations applies idempotent durable migrations for existing repositories.
func ApplyRepoMigrations(repoRoot string, opts InitOptions) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root cannot be empty")
	}

	migrationsDir := filepath.Join(repoRoot, repoDurableStateDir, "migrations")
	if err := ensureDir(migrationsDir, opts); err != nil {
		return fmt.Errorf("create migrations directory %s: %w", migrationsDir, err)
	}

	for _, migration := range repoMigrations {
		if strings.TrimSpace(migration.id) == "" {
			continue
		}
		markerPath := filepath.Join(migrationsDir, migration.id+".done")
		exists, err := pathExists(markerPath)
		if err != nil {
			return fmt.Errorf("check migration marker %s: %w", markerPath, err)
		}
		if exists {
			continue
		}
		if err := migration.apply(repoRoot, opts); err != nil {
			return fmt.Errorf("run migration %s: %w", migration.id, err)
		}
		if err := os.WriteFile(markerPath, []byte("ok\n"), 0o644); err != nil {
			return fmt.Errorf("write migration marker %s: %w", markerPath, err)
		}
	}

	return nil
}

func migrateConflictResolutionPrompt(repoRoot string, opts InitOptions) error {
	promptsDir := filepath.Join(repoRoot, "_governator", "prompts")
	if err := ensureDir(promptsDir, opts); err != nil {
		return fmt.Errorf("create prompts directory %s: %w", promptsDir, err)
	}

	targetPath := filepath.Join(promptsDir, conflictResolutionPromptName)
	exists, err := pathExists(targetPath)
	if err != nil {
		return fmt.Errorf("check conflict prompt %s: %w", targetPath, err)
	}
	if exists {
		return nil
	}
	if similarPromptExists(promptsDir, conflictResolutionPromptName) {
		return nil
	}

	data, err := templates.Read(conflictResolutionTemplatePath)
	if err != nil {
		return fmt.Errorf("read embedded template %s: %w", conflictResolutionTemplatePath, err)
	}
	if err := os.WriteFile(targetPath, data, 0o644); err != nil {
		return fmt.Errorf("write conflict prompt %s: %w", targetPath, err)
	}
	opts.logf("created planning prompt %s", repoRelativePath(repoRoot, targetPath))
	return nil
}

func similarPromptExists(promptsDir string, targetName string) bool {
	entries, err := os.ReadDir(promptsDir)
	if err != nil {
		return false
	}
	targetNormalized := normalizePromptStem(targetName)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if strings.EqualFold(name, targetName) {
			return true
		}
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "conflict") && strings.Contains(nameLower, "resolution") {
			return true
		}
		if normalizePromptStem(name) == targetNormalized {
			return true
		}
	}
	return false
}

func normalizePromptStem(name string) string {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	stem := strings.TrimSuffix(trimmed, strings.ToLower(filepath.Ext(trimmed)))
	builder := strings.Builder{}
	for _, r := range stem {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}
