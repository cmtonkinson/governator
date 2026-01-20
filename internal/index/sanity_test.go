// Tests for index sanity checks.
package index

import (
	"strings"
	"testing"
)

// TestSanityCheckHappyPath ensures valid indexes yield no warnings or errors.
func TestSanityCheckHappyPath(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateOpen,
				Role:  "planner",
			},
			{
				ID:           "task-2",
				Path:         "_governator/tasks/task-2.md",
				State:        TaskStateWorked,
				Role:         "builder",
				Dependencies: []string{"task-1"},
			},
		},
	}

	var warnings []string
	warn := func(message string) {
		warnings = append(warnings, message)
	}

	if err := SanityCheck(idx, warn); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", warnings)
	}
}

// TestSanityCheckDuplicateIDs verifies duplicate ids are reported as errors.
func TestSanityCheckDuplicateIDs(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskStateOpen,
				Role:  "planner",
			},
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1-copy.md",
				State: TaskStateOpen,
				Role:  "planner",
			},
		},
	}

	if err := SanityCheck(idx, nil); err == nil {
		t.Fatal("expected error for duplicate ids")
	} else if !strings.Contains(err.Error(), "duplicate task ids") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

// TestSanityCheckWarnings verifies warnings are emitted for unknown states and dependencies.
func TestSanityCheckWarnings(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Tasks: []Task{
			{
				ID:    "task-1",
				Path:  "_governator/tasks/task-1.md",
				State: TaskState("mystery"),
				Role:  "planner",
			},
			{
				ID:           "task-2",
				Path:         "_governator/tasks/task-2.md",
				State:        TaskStateOpen,
				Role:         "builder",
				Dependencies: []string{"task-99"},
			},
		},
	}

	var warnings []string
	warn := func(message string) {
		warnings = append(warnings, message)
	}

	if err := SanityCheck(idx, warn); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !warningsContain(warnings, "unknown state") {
		t.Fatalf("expected unknown state warning, got %v", warnings)
	}
	if !warningsContain(warnings, "missing dependency") {
		t.Fatalf("expected missing dependency warning, got %v", warnings)
	}
}

// warningsContain reports whether any warning contains the substring.
func warningsContain(warnings []string, substring string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, substring) {
			return true
		}
	}
	return false
}
