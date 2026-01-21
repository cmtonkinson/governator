package status

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

func TestSummaryString(t *testing.T) {
	tests := []struct {
		name     string
		summary  Summary
		expected string
	}{
		{
			name:     "empty summary",
			summary:  Summary{},
			expected: "tasks total=0 done=0 open=0 blocked=0",
		},
		{
			name: "mixed states",
			summary: Summary{
				Total:   10,
				Done:    3,
				Open:    5,
				Blocked: 2,
			},
			expected: "tasks total=10 done=3 open=5 blocked=2",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.summary.String()
			if result != tt.expected {
				t.Errorf("Summary.String() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestGetSummary(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "governator-status-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create the expected directory structure
	planDir := filepath.Join(tempDir, "_governator", "plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("Failed to create plan dir: %v", err)
	}

	// Create a test index with various task states
	testIndex := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{ID: "T-001", State: index.TaskStateDone},
			{ID: "T-002", State: index.TaskStateDone},
			{ID: "T-003", State: index.TaskStateOpen},
			{ID: "T-004", State: index.TaskStateOpen},
			{ID: "T-005", State: index.TaskStateOpen},
			{ID: "T-006", State: index.TaskStateBlocked},
			{ID: "T-007", State: index.TaskStateWorked},   // counts as open
			{ID: "T-008", State: index.TaskStateTested},   // counts as open
			{ID: "T-009", State: index.TaskStateConflict}, // counts as blocked
			{ID: "T-010", State: index.TaskStateResolved}, // counts as open
		},
	}

	// Write the test index to disk
	indexPath := filepath.Join(planDir, "task-index.json")
	if err := index.Save(indexPath, testIndex); err != nil {
		t.Fatalf("Failed to save test index: %v", err)
	}

	// Test GetSummary
	summary, err := GetSummary(tempDir)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	expected := Summary{
		Total:   10,
		Done:    2,  // T-001, T-002
		Open:    6,  // T-003, T-004, T-005, T-007, T-008, T-010
		Blocked: 2,  // T-006, T-009
	}

	if summary != expected {
		t.Errorf("GetSummary() = %+v, want %+v", summary, expected)
	}
}

func TestGetSummaryMissingIndex(t *testing.T) {
	// Create a temporary directory without an index file
	tempDir, err := os.MkdirTemp("", "governator-status-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Test GetSummary with missing index
	_, err = GetSummary(tempDir)
	if err == nil {
		t.Error("GetSummary() should fail when index file is missing")
	}
}

func TestGetSummaryEmptyIndex(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := os.MkdirTemp("", "governator-status-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create the expected directory structure
	planDir := filepath.Join(tempDir, "_governator", "plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		t.Fatalf("Failed to create plan dir: %v", err)
	}

	// Create an empty index
	emptyIndex := index.Index{
		SchemaVersion: 1,
		Tasks:         []index.Task{},
	}

	// Write the empty index to disk
	indexPath := filepath.Join(planDir, "task-index.json")
	if err := index.Save(indexPath, emptyIndex); err != nil {
		t.Fatalf("Failed to save empty index: %v", err)
	}

	// Test GetSummary
	summary, err := GetSummary(tempDir)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	expected := Summary{
		Total:   0,
		Done:    0,
		Open:    0,
		Blocked: 0,
	}

	if summary != expected {
		t.Errorf("GetSummary() = %+v, want %+v", summary, expected)
	}
}