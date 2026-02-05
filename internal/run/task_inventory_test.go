// Package run contains tests for task inventory integration.
package run

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

func TestTaskInventoryHappyPath(t *testing.T) {
	repo := testrepos.New(t)

	// Create task files
	tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}

	err := os.WriteFile(
		filepath.Join(tasksDir, "001-first.md"),
		[]byte("# First Task\n\nDo something."),
		0644,
	)
	if err != nil {
		t.Fatalf("write task file: %v", err)
	}

	// Create index and run inventory
	idx := &index.Index{Tasks: []index.Task{}}
	inventory := NewTaskInventory(repo.Root, idx)
	result, err := inventory.InventoryTasks()

	// Verify results
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TasksAdded != 1 {
		t.Fatalf("TasksAdded = %d, want 1", result.TasksAdded)
	}
	if len(idx.Tasks) != 1 {
		t.Fatalf("len(idx.Tasks) = %d, want 1", len(idx.Tasks))
	}
	if idx.Tasks[0].Title != "First Task" {
		t.Fatalf("title = %s, want 'First Task'", idx.Tasks[0].Title)
	}
	if idx.Tasks[0].ID != "001-first" {
		t.Fatalf("ID = %s, want '001-first'", idx.Tasks[0].ID)
	}
}

func TestTaskInventoryMultipleTasks(t *testing.T) {
	repo := testrepos.New(t)

	tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}

	// Create multiple task files
	tasks := map[string]string{
		"T-001-setup.md":     "# Setup Environment\n\nInitialize the project.",
		"T-002-implement.md": "# Implement Feature\n\nAdd the new feature.",
		"T-003-test.md":      "# Test Feature\n\nWrite tests.",
	}

	for filename, content := range tasks {
		err := os.WriteFile(filepath.Join(tasksDir, filename), []byte(content), 0644)
		if err != nil {
			t.Fatalf("write task file %s: %v", filename, err)
		}
	}

	idx := &index.Index{Tasks: []index.Task{}}
	inventory := NewTaskInventory(repo.Root, idx)
	result, err := inventory.InventoryTasks()

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.TasksAdded != 3 {
		t.Fatalf("TasksAdded = %d, want 3", result.TasksAdded)
	}
	if len(idx.Tasks) != 3 {
		t.Fatalf("len(idx.Tasks) = %d, want 3", len(idx.Tasks))
	}

	// Verify specific tasks
	taskByID := make(map[string]index.Task)
	for _, task := range idx.Tasks {
		taskByID[task.ID] = task
	}

	if task, ok := taskByID["T-001-setup"]; !ok {
		t.Fatalf("T-001-setup not found in index")
	} else if task.Title != "Setup Environment" {
		t.Fatalf("T-001-setup title = %s, want 'Setup Environment'", task.Title)
	}

	if task, ok := taskByID["T-002-implement"]; !ok {
		t.Fatalf("T-002-implement not found in index")
	} else if task.Title != "Implement Feature" {
		t.Fatalf("T-002-implement title = %s, want 'Implement Feature'", task.Title)
	}

	if task, ok := taskByID["T-003-test"]; !ok {
		t.Fatalf("T-003-test not found in index")
	} else if task.Title != "Test Feature" {
		t.Fatalf("T-003-test title = %s, want 'Test Feature'", task.Title)
	}
}

func TestTaskInventoryIdempotent(t *testing.T) {
	repo := testrepos.New(t)

	tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}

	err := os.WriteFile(
		filepath.Join(tasksDir, "001-task.md"),
		[]byte("# Test Task\n\nContent."),
		0644,
	)
	if err != nil {
		t.Fatalf("write task file: %v", err)
	}

	idx := &index.Index{Tasks: []index.Task{}}
	inventory := NewTaskInventory(repo.Root, idx)

	// First run
	result1, err := inventory.InventoryTasks()
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if result1.TasksAdded != 1 {
		t.Fatalf("first run TasksAdded = %d, want 1", result1.TasksAdded)
	}

	// Second run (should not add duplicates)
	result2, err := inventory.InventoryTasks()
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if result2.TasksAdded != 0 {
		t.Fatalf("second run TasksAdded = %d, want 0", result2.TasksAdded)
	}
	if len(idx.Tasks) != 1 {
		t.Fatalf("after second run len(idx.Tasks) = %d, want 1", len(idx.Tasks))
	}
}

func TestTaskInventoryTitleExtraction(t *testing.T) {
	repo := testrepos.New(t)

	tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}

	tests := []struct {
		name      string
		filename  string
		content   string
		wantTitle string
	}{
		{
			name:      "h1_at_start",
			filename:  "001-h1-start.md",
			content:   "# Title at Start\n\nContent here.",
			wantTitle: "Title at Start",
		},
		{
			name:      "h1_after_whitespace",
			filename:  "002-h1-whitespace.md",
			content:   "\n\n# Title After Whitespace\n\nContent.",
			wantTitle: "Title After Whitespace",
		},
		{
			name:      "no_h1_uses_untitled",
			filename:  "003-no-h1.md",
			content:   "## This is H2\n\nNo H1 here.",
			wantTitle: "Untitled Task",
		},
		{
			name:      "multiple_h1_uses_first",
			filename:  "004-multiple-h1.md",
			content:   "# First Title\n\nSome content.\n\n# Second Title\n\nMore content.",
			wantTitle: "First Title",
		},
		{
			name:      "h1_with_inline_code",
			filename:  "005-inline-code.md",
			content:   "# Task with `code` inline\n\nContent.",
			wantTitle: "Task with `code` inline",
		},
		{
			name:      "h1_with_special_chars",
			filename:  "006-special-chars.md",
			content:   "# Task: Setup & Configure (v2.0)\n\nContent.",
			wantTitle: "Setup & Configure (v2.0)",
		},
		{
			name:      "empty_file_uses_untitled",
			filename:  "007-empty.md",
			content:   "",
			wantTitle: "Untitled Task",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := os.WriteFile(filepath.Join(tasksDir, tt.filename), []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("write task file: %v", err)
			}

			idx := &index.Index{Tasks: []index.Task{}}
			inventory := NewTaskInventory(repo.Root, idx)
			_, err = inventory.InventoryTasks()
			if err != nil {
				t.Fatalf("inventory error: %v", err)
			}

			if len(idx.Tasks) == 0 {
				t.Fatalf("no tasks added")
			}

			// Find the task we just added
			var foundTask *index.Task
			for i := range idx.Tasks {
				if strings.Contains(idx.Tasks[i].ID, strings.TrimSuffix(tt.filename, ".md")) {
					foundTask = &idx.Tasks[i]
					break
				}
			}

			if foundTask == nil {
				t.Fatalf("task not found in index")
			}

			if foundTask.Title != tt.wantTitle {
				t.Fatalf("title = %q, want %q", foundTask.Title, tt.wantTitle)
			}

			// Clean up for next test
			os.Remove(filepath.Join(tasksDir, tt.filename))
		})
	}
}

func TestTaskInventoryDefaultValues(t *testing.T) {
	repo := testrepos.New(t)

	tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}

	err := os.WriteFile(
		filepath.Join(tasksDir, "001-task.md"),
		[]byte("# Test Task\n\nContent."),
		0644,
	)
	if err != nil {
		t.Fatalf("write task file: %v", err)
	}

	idx := &index.Index{Tasks: []index.Task{}}
	inventory := NewTaskInventory(repo.Root, idx)
	_, err = inventory.InventoryTasks()
	if err != nil {
		t.Fatalf("inventory error: %v", err)
	}

	if len(idx.Tasks) != 1 {
		t.Fatalf("len(idx.Tasks) = %d, want 1", len(idx.Tasks))
	}

	task := idx.Tasks[0]

	// Verify default values
	if task.Kind != index.TaskKindExecution {
		t.Fatalf("Kind = %s, want %s", task.Kind, index.TaskKindExecution)
	}
	if task.State != index.TaskStateBacklog {
		t.Fatalf("State = %s, want %s", task.State, index.TaskStateBacklog)
	}
	if task.Role != index.Role("default") {
		t.Fatalf("Role = %s, want 'default'", task.Role)
	}
	if task.Retries.MaxAttempts != 3 {
		t.Fatalf("Retries.MaxAttempts = %d, want 3", task.Retries.MaxAttempts)
	}
	if task.Order != 1 {
		t.Fatalf("Order = %d, want 1", task.Order)
	}
	if task.Attempts.Total != 0 {
		t.Fatalf("Attempts.Total = %d, want 0", task.Attempts.Total)
	}
	if task.Attempts.Failed != 0 {
		t.Fatalf("Attempts.Failed = %d, want 0", task.Attempts.Failed)
	}
}

func TestTaskInventoryOrderIncrement(t *testing.T) {
	repo := testrepos.New(t)

	tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}

	// Create three tasks
	for i := 1; i <= 3; i++ {
		filename := filepath.Join(tasksDir, fmt.Sprintf("00%d-task.md", i))
		err := os.WriteFile(filename, []byte(fmt.Sprintf("# Task %d\n\nContent.", i)), 0644)
		if err != nil {
			t.Fatalf("write task file %d: %v", i, err)
		}
	}

	idx := &index.Index{Tasks: []index.Task{}}
	inventory := NewTaskInventory(repo.Root, idx)
	_, err := inventory.InventoryTasks()
	if err != nil {
		t.Fatalf("inventory error: %v", err)
	}

	if len(idx.Tasks) != 3 {
		t.Fatalf("len(idx.Tasks) = %d, want 3", len(idx.Tasks))
	}

	// Verify order increments
	for i, task := range idx.Tasks {
		expectedOrder := i + 1
		if task.Order != expectedOrder {
			t.Fatalf("task[%d].Order = %d, want %d", i, task.Order, expectedOrder)
		}
	}
}

func TestTaskInventoryErrorHandling(t *testing.T) {
	t.Run("missing_tasks_directory", func(t *testing.T) {
		repo := testrepos.New(t)

		// Don't create tasks directory
		idx := &index.Index{Tasks: []index.Task{}}
		inventory := NewTaskInventory(repo.Root, idx)
		_, err := inventory.InventoryTasks()

		if err == nil {
			t.Fatalf("expected error for missing tasks directory")
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("error message should mention directory does not exist, got: %v", err)
		}
	})

	t.Run("empty_tasks_directory", func(t *testing.T) {
		repo := testrepos.New(t)

		tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}

		idx := &index.Index{Tasks: []index.Task{}}
		inventory := NewTaskInventory(repo.Root, idx)
		_, err := inventory.InventoryTasks()

		if err == nil {
			t.Fatalf("expected error for empty tasks directory")
		}
		if !strings.Contains(err.Error(), "no task markdown files") {
			t.Fatalf("error message should mention no markdown files, got: %v", err)
		}
	})

	t.Run("unreadable_task_file", func(t *testing.T) {
		if os.Getuid() == 0 {
			t.Skip("skipping permission test when running as root")
		}

		repo := testrepos.New(t)

		tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}

		unreadableFile := filepath.Join(tasksDir, "001-unreadable.md")
		err := os.WriteFile(unreadableFile, []byte("# Unreadable\n\nContent."), 0644)
		if err != nil {
			t.Fatalf("write task file: %v", err)
		}

		// Make file unreadable
		if err := os.Chmod(unreadableFile, 0000); err != nil {
			t.Fatalf("chmod failed: %v", err)
		}
		defer os.Chmod(unreadableFile, 0644) // Restore for cleanup

		idx := &index.Index{Tasks: []index.Task{}}
		inventory := NewTaskInventory(repo.Root, idx)
		result, err := inventory.InventoryTasks()

		// Should not return a hard error, but should have errors in result
		if err != nil {
			t.Fatalf("unexpected hard error: %v", err)
		}
		if len(result.Errors) == 0 {
			t.Fatalf("expected errors in result for unreadable file")
		}
		if result.TasksAdded != 0 {
			t.Fatalf("TasksAdded = %d, want 0 (file unreadable)", result.TasksAdded)
		}
	})

	t.Run("malformed_task_file_empty", func(t *testing.T) {
		repo := testrepos.New(t)

		tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}

		// Empty file is still valid markdown, just has no title
		emptyFile := filepath.Join(tasksDir, "001-empty.md")
		err := os.WriteFile(emptyFile, []byte(""), 0644)
		if err != nil {
			t.Fatalf("write task file: %v", err)
		}

		idx := &index.Index{Tasks: []index.Task{}}
		inventory := NewTaskInventory(repo.Root, idx)
		result, err := inventory.InventoryTasks()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Empty file should be processed successfully with "Untitled Task"
		if result.TasksAdded != 1 {
			t.Fatalf("TasksAdded = %d, want 1 (empty file creates untitled task)", result.TasksAdded)
		}
		if idx.Tasks[0].Title != "Untitled Task" {
			t.Fatalf("title = %s, want 'Untitled Task'", idx.Tasks[0].Title)
		}
	})
}

func TestTaskInventoryEdgeCases(t *testing.T) {
	t.Run("subdirectories_ignored", func(t *testing.T) {
		repo := testrepos.New(t)

		tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}

		// Create a task file
		err := os.WriteFile(filepath.Join(tasksDir, "001-task.md"), []byte("# Task\n\nContent."), 0644)
		if err != nil {
			t.Fatalf("write task file: %v", err)
		}

		// Create a subdirectory with a markdown file
		subdir := filepath.Join(tasksDir, "archive")
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatalf("create subdir: %v", err)
		}
		err = os.WriteFile(filepath.Join(subdir, "002-archived.md"), []byte("# Archived\n\nOld task."), 0644)
		if err != nil {
			t.Fatalf("write archived task: %v", err)
		}

		idx := &index.Index{Tasks: []index.Task{}}
		inventory := NewTaskInventory(repo.Root, idx)
		result, err := inventory.InventoryTasks()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only the top-level task should be counted
		if result.TasksAdded != 1 {
			t.Fatalf("TasksAdded = %d, want 1 (subdirectories should be ignored)", result.TasksAdded)
		}
	})

	t.Run("non_markdown_files_ignored", func(t *testing.T) {
		repo := testrepos.New(t)

		tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}

		// Create a markdown file
		err := os.WriteFile(filepath.Join(tasksDir, "001-task.md"), []byte("# Task\n\nContent."), 0644)
		if err != nil {
			t.Fatalf("write task file: %v", err)
		}

		// Create non-markdown files
		err = os.WriteFile(filepath.Join(tasksDir, "notes.txt"), []byte("Notes"), 0644)
		if err != nil {
			t.Fatalf("write txt file: %v", err)
		}
		err = os.WriteFile(filepath.Join(tasksDir, "data.json"), []byte("{}"), 0644)
		if err != nil {
			t.Fatalf("write json file: %v", err)
		}

		idx := &index.Index{Tasks: []index.Task{}}
		inventory := NewTaskInventory(repo.Root, idx)
		result, err := inventory.InventoryTasks()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Only the markdown file should be counted
		if result.TasksAdded != 1 {
			t.Fatalf("TasksAdded = %d, want 1 (non-markdown files should be ignored)", result.TasksAdded)
		}
	})

	t.Run("filenames_with_spaces", func(t *testing.T) {
		repo := testrepos.New(t)

		tasksDir := filepath.Join(repo.Root, "_governator", "tasks")
		if err := os.MkdirAll(tasksDir, 0755); err != nil {
			t.Fatalf("create tasks dir: %v", err)
		}

		filename := "001 task with spaces.md"
		err := os.WriteFile(filepath.Join(tasksDir, filename), []byte("# Task With Spaces\n\nContent."), 0644)
		if err != nil {
			t.Fatalf("write task file: %v", err)
		}

		idx := &index.Index{Tasks: []index.Task{}}
		inventory := NewTaskInventory(repo.Root, idx)
		result, err := inventory.InventoryTasks()

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.TasksAdded != 1 {
			t.Fatalf("TasksAdded = %d, want 1", result.TasksAdded)
		}
		if idx.Tasks[0].ID != "001 task with spaces" {
			t.Fatalf("ID = %s, want '001 task with spaces'", idx.Tasks[0].ID)
		}
	})
}
