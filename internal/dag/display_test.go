package dag

import (
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

func TestGetSummary_Basic(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{
				ID:           "001-auth",
				Title:        "Implement authentication",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateTriaged,
				Dependencies: []string{},
				Order:        10,
			},
			{
				ID:           "002-login",
				Title:        "Add login page",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateTriaged,
				Dependencies: []string{"001-auth"},
				Order:        20,
			},
			{
				ID:           "003-test",
				Title:        "Add tests",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateMerged,
				Dependencies: []string{"001-auth", "002-login"},
				Order:        30,
			},
		},
	}

	summary := GetSummary(idx)

	if summary.TotalTasks != 3 {
		t.Errorf("expected 3 tasks, got %d", summary.TotalTasks)
	}
	if summary.InProgress != 2 {
		t.Errorf("expected 2 in-progress, got %d", summary.InProgress)
	}
	if summary.Merged != 1 {
		t.Errorf("expected 1 merged, got %d", summary.Merged)
	}
	if len(summary.Tasks) != 3 {
		t.Errorf("expected 3 task rows, got %d", len(summary.Tasks))
	}

	// Check first task (001) - no dependencies, blocks 002 and 003
	task001 := summary.Tasks[0]
	if task001.ID != "001" {
		t.Errorf("expected ID 001, got %s", task001.ID)
	}
	if task001.DependsOn != "-" {
		t.Errorf("expected no dependencies, got %s", task001.DependsOn)
	}
	if !strings.Contains(task001.Blocks, "002") || !strings.Contains(task001.Blocks, "003") {
		t.Errorf("expected to block 002 and 003, got %s", task001.Blocks)
	}

	// Check second task (002) - depends on 001, blocks 003
	task002 := summary.Tasks[1]
	if task002.ID != "002" {
		t.Errorf("expected ID 002, got %s", task002.ID)
	}
	if task002.DependsOn != "001" {
		t.Errorf("expected to depend on 001, got %s", task002.DependsOn)
	}
	if task002.Blocks != "003" {
		t.Errorf("expected to block 003, got %s", task002.Blocks)
	}

	// Check third task (003) - depends on 001 and 002, blocks nothing
	task003 := summary.Tasks[2]
	if task003.ID != "003" {
		t.Errorf("expected ID 003, got %s", task003.ID)
	}
	if !strings.Contains(task003.DependsOn, "001") || !strings.Contains(task003.DependsOn, "002") {
		t.Errorf("expected to depend on 001 and 002, got %s", task003.DependsOn)
	}
	if task003.Blocks != "-" {
		t.Errorf("expected to block nothing, got %s", task003.Blocks)
	}
}

func TestGetSummary_Empty(t *testing.T) {
	idx := index.Index{Tasks: []index.Task{}}
	summary := GetSummary(idx)

	if summary.TotalTasks != 0 {
		t.Errorf("expected 0 tasks, got %d", summary.TotalTasks)
	}
	if len(summary.Tasks) != 0 {
		t.Errorf("expected 0 task rows, got %d", len(summary.Tasks))
	}
}

func TestGetSummary_PlanningTasksIgnored(t *testing.T) {
	idx := index.Index{
		Tasks: []index.Task{
			{
				ID:    "planning",
				Kind:  index.TaskKindPlanning,
				State: index.TaskStateTriaged,
			},
			{
				ID:           "001-work",
				Kind:         index.TaskKindExecution,
				State:        index.TaskStateTriaged,
				Dependencies: []string{},
			},
		},
	}

	summary := GetSummary(idx)

	if summary.TotalTasks != 2 {
		t.Errorf("expected 2 total tasks, got %d", summary.TotalTasks)
	}
	if len(summary.Tasks) != 1 {
		t.Errorf("expected 1 execution task row, got %d", len(summary.Tasks))
	}
	if summary.Tasks[0].ID != "001" {
		t.Errorf("expected execution task, got %s", summary.Tasks[0].ID)
	}
}

func TestExtractNumericID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"001-implement-auth", "001"},
		{"042-add-feature", "042"},
		{"123", "123"},
		{"no-prefix", "no"},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractNumericID(tt.input)
		if result != tt.expected {
			t.Errorf("extractNumericID(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		width    int
		expected string
	}{
		{"short", 10, "short"},
		{"exactly ten", 11, "exactly ten"},
		{"this is a very long string", 15, "this is a ve..."},
		{"short", 3, "sho"},
		{"toolong", 5, "to..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.width)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.width, result, tt.expected)
		}
	}
}
