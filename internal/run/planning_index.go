// Package run seeds and maintains the planning task index entries.
package run

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
)

const taskIndexSchemaVersion = 1

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

	planning := newPlanningTask()
	tasks := planningTasks(planning)

	digestsMap, err := digests.Compute(repoRoot)
	if err != nil {
		return fmt.Errorf("compute digests: %w", err)
	}

	idx := index.Index{
		SchemaVersion: taskIndexSchemaVersion,
		Digests:       digestsMap,
		Tasks:         tasks,
	}
	if err := index.Save(indexPath, idx); err != nil {
		return fmt.Errorf("save task index: %w", err)
	}
	return nil
}

// UpdatePlanningIndex refreshes digests and marks the completed planning step.
func UpdatePlanningIndex(worktreePath string, step workstreamStep) error {
	if strings.TrimSpace(worktreePath) == "" {
		return fmt.Errorf("worktree path is required")
	}
	indexPath := filepath.Join(worktreePath, indexFilePath)
	idx, err := index.Load(indexPath)
	if err != nil {
		return fmt.Errorf("load task index: %w", err)
	}

	digestsMap, err := digests.Compute(worktreePath)
	if err != nil {
		return fmt.Errorf("compute digests: %w", err)
	}
	idx.Digests = digestsMap

	taskID := planningTaskID(step)
	for i := range idx.Tasks {
		if idx.Tasks[i].ID != taskID {
			continue
		}
		idx.Tasks[i].State = index.TaskStateMerged
		idx.Tasks[i].PID = 0
		break
	}

	if err := index.Save(indexPath, idx); err != nil {
		return fmt.Errorf("save task index: %w", err)
	}
	return commitPlanningIndex(worktreePath, step.title())
}

// planningTasks converts the planning workstream definition into index tasks.
func planningTasks(task planningTask) []index.Task {
	if len(task.ordered) == 0 {
		return nil
	}
	tasks := make([]index.Task, 0, len(task.ordered))
	var previousID string
	for _, step := range task.ordered {
		taskID := planningTaskID(step)
		deps := []string{}
		if previousID != "" {
			deps = []string{previousID}
		}
		tasks = append(tasks, index.Task{
			ID:           taskID,
			Title:        step.title(),
			Path:         step.promptPath,
			Kind:         index.TaskKindPlanning,
			State:        index.TaskStateBacklog,
			Role:         step.role,
			Dependencies: deps,
			Retries:      index.RetryPolicy{MaxAttempts: 1},
			Attempts:     index.AttemptCounters{},
			Order:        step.phase.Number() * 10,
			Overlap:      []string{},
		})
		previousID = taskID
	}
	return tasks
}

// planningTaskID builds the stable, worktree-safe task id for a planning step.
func planningTaskID(step workstreamStep) string {
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
