// Package planner tests task file generation.
package planner

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteTaskFilesHappyPath(t *testing.T) {
	root := t.TempDir()
	tasks := []PlannedTask{
		{
			ID:           "task-01",
			Title:        "Implement Index IO",
			Summary:      "Write the task index loader and saver.",
			Role:         "engineer",
			Dependencies: []string{},
			Order:        10,
			Overlap:      []string{"index"},
			AcceptanceCriteria: []string{
				"Index load and save functions are available.",
			},
			Tests: []string{
				"Unit tests cover load and save happy paths.",
			},
		},
	}

	result, err := WriteTaskFiles(root, tasks, TaskFileOptions{})
	if err != nil {
		t.Fatalf("write task files: %v", err)
	}
	if len(result.Written) != 1 {
		t.Fatalf("expected 1 written file, got %d", len(result.Written))
	}
	if len(result.Skipped) != 0 {
		t.Fatalf("expected 0 skipped files, got %d", len(result.Skipped))
	}

	path := filepath.Join(root, "_governator", "tasks", "task-01-implement-index-io.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read task file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "# Task: Implement Index IO") {
		t.Fatalf("expected task title in content")
	}
	if !strings.Contains(content, "task: task-01") {
		t.Fatalf("expected task id in front matter")
	}
	if !strings.Contains(content, "depends_on: []") {
		t.Fatalf("expected dependencies in front matter")
	}
	if !strings.Contains(content, "Write the task index loader and saver.") {
		t.Fatalf("expected summary in objective section")
	}
	if !strings.Contains(content, "Index load and save functions are available.") {
		t.Fatalf("expected acceptance criteria in content")
	}
	if !strings.Contains(content, "## Tests") {
		t.Fatalf("expected tests section in content")
	}
	if !strings.Contains(content, "Unit tests cover load and save happy paths.") {
		t.Fatalf("expected tests content in task file")
	}
}

func TestWriteTaskFilesDoesNotOverwrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "_governator", "tasks", "task-02-already-there.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create tasks dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write existing task file: %v", err)
	}

	tasks := []PlannedTask{
		{
			ID:           "task-02",
			Title:        "Already There",
			Summary:      "Should not overwrite existing task file.",
			Role:         "engineer",
			Dependencies: []string{},
			Order:        20,
			Overlap:      []string{},
			AcceptanceCriteria: []string{
				"Existing task file remains unchanged.",
			},
			Tests: []string{"N/A"},
		},
	}

	result, err := WriteTaskFiles(root, tasks, TaskFileOptions{})
	if err != nil {
		t.Fatalf("write task files: %v", err)
	}
	if len(result.Written) != 0 {
		t.Fatalf("expected 0 written files, got %d", len(result.Written))
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected 1 skipped file, got %d", len(result.Skipped))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing task file: %v", err)
	}
	if string(data) != "existing" {
		t.Fatalf("expected existing content to remain")
	}
}
