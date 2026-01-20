// Package planner builds task index entries from planner output.
package planner

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

// IndexTaskOptions configures task index creation defaults.
type IndexTaskOptions struct {
	MaxAttempts int
}

// BuildIndexTasks maps planner tasks into task index entries.
func BuildIndexTasks(tasks []PlannedTask, options IndexTaskOptions) ([]index.Task, error) {
	if len(tasks) == 0 {
		return nil, errors.New("tasks are required")
	}
	if options.MaxAttempts < 0 {
		return nil, errors.New("max attempts must be zero or positive")
	}

	seenIDs := map[string]struct{}{}
	seenPaths := map[string]struct{}{}
	indexTasks := make([]index.Task, 0, len(tasks))

	for _, task := range tasks {
		if err := validatePlannedTaskForIndex(task); err != nil {
			return nil, err
		}

		id := strings.TrimSpace(task.ID)
		title := strings.TrimSpace(task.Title)
		role := strings.TrimSpace(task.Role)
		path := indexTaskPath(id, title)

		if _, ok := seenIDs[id]; ok {
			return nil, fmt.Errorf("duplicate task id %q", id)
		}
		seenIDs[id] = struct{}{}

		if _, ok := seenPaths[path]; ok {
			return nil, fmt.Errorf("task path collision for %s", path)
		}
		seenPaths[path] = struct{}{}

		indexTasks = append(indexTasks, index.Task{
			ID:           id,
			Title:        title,
			Path:         path,
			State:        index.TaskStateOpen,
			Role:         index.Role(role),
			Dependencies: normalizeStringList(task.Dependencies),
			Retries: index.RetryPolicy{
				MaxAttempts: options.MaxAttempts,
			},
			Attempts: index.AttemptCounters{},
			Order:    task.Order,
			Overlap:  normalizeStringList(task.Overlap),
		})
	}

	return indexTasks, nil
}

// validatePlannedTaskForIndex ensures required planner task fields are present.
func validatePlannedTaskForIndex(task PlannedTask) error {
	if strings.TrimSpace(task.ID) == "" {
		return errors.New("task id is required")
	}
	if strings.TrimSpace(task.Title) == "" {
		return errors.New("task title is required")
	}
	if strings.TrimSpace(task.Role) == "" {
		return errors.New("task role is required")
	}
	if task.Order <= 0 {
		return errors.New("task order is required")
	}
	return nil
}

// indexTaskPath builds the index task path from the planned task metadata.
func indexTaskPath(id string, title string) string {
	return filepath.ToSlash(filepath.Join(tasksDirName, taskFileName(id, title)))
}

// normalizeStringList trims values and drops empty entries.
func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	cleaned := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}
	if len(cleaned) == 0 {
		return []string{}
	}
	return cleaned
}
