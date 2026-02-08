// Package run provides task inventory functionality for planning completion.
package run

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cmtonkinson/governator/internal/index"
)

// TaskInventoryResult captures the outcome of task inventory.
type TaskInventoryResult struct {
	TasksAdded int
	Errors     []error
}

// TaskInventory performs inventory of task files and updates the index.
type TaskInventory struct {
	repoRoot string
	idx      *index.Index
}

// NewTaskInventory creates a new task inventory for the given repository.
func NewTaskInventory(repoRoot string, idx *index.Index) *TaskInventory {
	return &TaskInventory{repoRoot: repoRoot, idx: idx}
}

// InventoryTasks scans the tasks directory and adds new tasks to the index.
func (inventory *TaskInventory) InventoryTasks() (TaskInventoryResult, error) {
	result := TaskInventoryResult{}

	tasksDir := filepath.Join(inventory.repoRoot, "_governator", "tasks")

	// Check if tasks directory exists
	if _, err := os.Stat(tasksDir); err != nil {
		if os.IsNotExist(err) {
			return result, fmt.Errorf("tasks directory does not exist: %s", tasksDir)
		}
		return result, fmt.Errorf("stat tasks directory: %w", err)
	}

	// Read all task files
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		return result, fmt.Errorf("read tasks directory: %w", err)
	}

	// Filter and process markdown files
	var taskFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.HasSuffix(entry.Name(), ".md") {
			taskFiles = append(taskFiles, entry.Name())
		}
	}

	if len(taskFiles) == 0 {
		return result, fmt.Errorf("no task markdown files found in %s", tasksDir)
	}

	existingPaths := make(map[string]struct{}, len(inventory.idx.Tasks))
	for _, existingTask := range inventory.idx.Tasks {
		canonicalPath := canonicalTaskPath(existingTask.Path)
		if canonicalPath == "" {
			continue
		}
		existingPaths[canonicalPath] = struct{}{}
	}

	sort.Strings(taskFiles)

	// Process each task file
	for _, filename := range taskFiles {
		filePath := filepath.Join(tasksDir, filename)
		relativePath, err := filepath.Rel(inventory.repoRoot, filePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("compute relative path for %s: %w", filename, err))
			continue
		}

		canonicalPath := canonicalTaskPath(relativePath)
		if _, exists := existingPaths[canonicalPath]; exists {
			continue
		}

		// Parse task title and initialize index-owned execution defaults.
		task, err := inventory.parseTaskFile(filePath, canonicalPath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("parse task %s: %w", filename, err))
			continue
		}
		inventory.idx.Tasks = append(inventory.idx.Tasks, task)
		existingPaths[canonicalPath] = struct{}{}
		result.TasksAdded++
	}

	return result, nil
}

// parseTaskFile extracts task title from a markdown file and builds an execution task.
func (inventory *TaskInventory) parseTaskFile(filePath, taskPath string) (index.Task, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return index.Task{}, fmt.Errorf("read task file: %w", err)
	}

	return index.Task{
		ID:       taskIDFromPath(taskPath),
		Title:    extractTitleFromMarkdown(string(content)),
		Path:     taskPath,
		Kind:     index.TaskKindExecution,
		State:    index.TaskStateBacklog,
		Role:     index.Role("default"),
		Retries:  index.RetryPolicy{MaxAttempts: 3},
		Attempts: index.AttemptCounters{Total: 0, Failed: 0},
		Order:    len(inventory.idx.Tasks) + 1,
	}, nil
}

// canonicalTaskPath normalizes a task path for identity comparisons.
func canonicalTaskPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	return filepath.ToSlash(filepath.Clean(trimmed))
}

// taskIDFromPath derives the workstream identifier from the canonical task path.
func taskIDFromPath(taskPath string) string {
	base := filepath.Base(taskPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// extractTitleFromMarkdown extracts the title from markdown content.
func extractTitleFromMarkdown(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# ") {
			title := strings.TrimPrefix(trimmed, "# ")
			// Strip redundant "Task: " prefix that's common in markdown headers
			title = strings.TrimPrefix(title, "Task: ")
			return title
		}
	}
	return "Untitled Task"
}
