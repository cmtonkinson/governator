// Package run provides task inventory functionality for planning completion.
package run

import (
	"fmt"
	"os"
	"path/filepath"
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
	
	// Process each task file
	for _, filename := range taskFiles {
		filePath := filepath.Join(tasksDir, filename)
		
		// Parse task metadata from markdown
		task, err := inventory.parseTaskFile(filePath)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("parse task %s: %w", filename, err))
			continue
		}
		
		// Check if task already exists in index
		exists := false
		for _, existingTask := range inventory.idx.Tasks {
			if existingTask.Path == task.Path {
				exists = true
				break
			}
		}
		
		if !exists {
			// Add new task to index
			inventory.idx.Tasks = append(inventory.idx.Tasks, task)
			result.TasksAdded++
		}
	}
	
	return result, nil
}

// parseTaskFile extracts task metadata from a markdown file.
func (inventory *TaskInventory) parseTaskFile(filePath string) (index.Task, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return index.Task{}, fmt.Errorf("read task file: %w", err)
	}
	
	// Basic parsing - in a real implementation, this would parse frontend matter or
	// specific markdown sections to extract title, dependencies, etc.
	
	// For now, we'll create a basic task with default values
	relativePath, err := filepath.Rel(inventory.repoRoot, filePath)
	if err != nil {
		return index.Task{}, fmt.Errorf("compute relative path: %w", err)
	}
	
	taskID := strings.TrimSuffix(filepath.Base(filePath), ".md")
	
	return index.Task{
		ID:       taskID,
		Title:    extractTitleFromMarkdown(string(content)),
		Path:     relativePath,
		Kind:     index.TaskKindExecution,
		State:    index.TaskStateBacklog,
		Role:     index.Role("default"),
		Retries:  index.RetryPolicy{MaxAttempts: 3},
		Attempts: index.AttemptCounters{Total: 0, Failed: 0},
		Order:    len(inventory.idx.Tasks) + 1, // Simple ordering
	}, nil
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