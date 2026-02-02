// Package run seeds and maintains the planning task index entries.
package run

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
)

const (
	taskIndexSchemaVersion = 1
	planningIndexTaskID    = "planning"
)

// SeedPlanningIndex writes the planning task index on init when it is missing.
func SeedPlanningIndex(repoRoot string) error {
	if strings.TrimSpace(repoRoot) == "" {
		return fmt.Errorf("repo root is required")
	}
	indexPath := filepath.Join(repoRoot, indexFilePath)
	if _, err := os.Stat(indexPath); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat task index: %w", err)
	}

	spec, err := LoadPlanningSpec(repoRoot)
	if err != nil {
		return fmt.Errorf("load planning spec: %w", err)
	}
	if len(spec.Steps) == 0 {
		return fmt.Errorf("planning spec requires at least one step")
	}
	firstStepID := strings.TrimSpace(spec.Steps[0].ID)
	if firstStepID == "" {
		return fmt.Errorf("planning spec first step id is required")
	}
	tasks := []index.Task{
		{
			ID:       planningIndexTaskID,
			Title:    "Planning",
			Path:     planningSpecFilePath,
			Kind:     index.TaskKindPlanning,
			State:    index.TaskStateTriaged,
			Retries:  index.RetryPolicy{MaxAttempts: 1},
			Attempts: index.AttemptCounters{},
		},
	}

	digestsMap, err := digests.Compute(repoRoot)
	if err != nil {
		return fmt.Errorf("compute digests: %w", err)
	}

	idx := index.Index{
		SchemaVersion: taskIndexSchemaVersion,
		Digests:       digestsMap,
		Tasks:         tasks,
	}
	updatePlanningTaskState(&idx, firstStepID)
	if err := index.Save(indexPath, idx); err != nil {
		return fmt.Errorf("save task index: %w", err)
	}
	return nil
}

// UpdatePlanningIndex refreshes digests after a completed planning step.
func UpdatePlanningIndex(worktreePath string, step workstreamStep) error {
	if strings.TrimSpace(worktreePath) == "" {
		return fmt.Errorf("worktree path is required")
	}
	indexPath := filepath.Join(worktreePath, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		// If index doesn't exist in worktree, create initial index
		if errors.Is(err, os.ErrNotExist) {
			idx = index.Index{
				SchemaVersion: taskIndexSchemaVersion,
				Tasks: []index.Task{
					{
						ID:       planningIndexTaskID,
						Title:    "Planning",
						Path:     planningSpecFilePath,
						Kind:     index.TaskKindPlanning,
						State:    index.TaskStateTriaged,
						Retries:  index.RetryPolicy{MaxAttempts: 1},
						Attempts: index.AttemptCounters{},
					},
				},
			}
			// Ensure directory exists
			indexDir := filepath.Dir(indexPath)
			if err := os.MkdirAll(indexDir, 0o755); err != nil {
				return fmt.Errorf("create index directory: %w", err)
			}
		} else {
			return fmt.Errorf("load task index: %w", err)
		}
	}

	digestsMap, err := digests.Compute(worktreePath)
	if err != nil {
		return fmt.Errorf("compute digests: %w", err)
	}
	idx.Digests = digestsMap

	if err := index.Save(indexPath, idx); err != nil {
		return fmt.Errorf("save task index: %w", err)
	}
	return commitPlanningIndex(worktreePath, step.title())
}

// planningStepWorkstreamID builds the stable, worktree-safe id for a planning step workstream.
func planningStepWorkstreamID(step workstreamStep) string {
	return fmt.Sprintf("planning-%s", step.name)
}

// commitPlanningIndex records index updates on the planning branch when needed.
func commitPlanningIndex(worktreePath string, title string) error {
	status, err := runGitOutput(worktreePath, "status", "--porcelain", "--", indexFilePath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if err := runGit(worktreePath, "add", "--", indexFilePath); err != nil {
		return err
	}
	subject := strings.TrimSpace(title)
	if subject == "" {
		subject = "planning"
	}
	message := fmt.Sprintf("[planning] %s index", subject)
	return runGitWithEnv(worktreePath, []string{
		"GIT_AUTHOR_NAME=Governator CLI",
		"GIT_AUTHOR_EMAIL=governator@localhost",
		"GIT_COMMITTER_NAME=Governator CLI",
		"GIT_COMMITTER_EMAIL=governator@localhost",
	}, "commit", "-m", message)
}
