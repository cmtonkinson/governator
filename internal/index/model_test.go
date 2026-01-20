// Tests for the task index data model.
package index

import (
	"encoding/json"
	"reflect"
	"testing"
)

const exampleIndexJSON = `{
  "schema_version": 1,
  "digests": {
    "governator_md": "sha256:ef3a4c...",
    "planning_docs": {}
  },
  "tasks": [
    {
      "id": "task-01",
      "title": "Initialize repo",
      "path": "_governator/tasks/task-01-initialize.md",
      "state": "open",
      "role": "planner",
      "dependencies": [],
      "retries": {
        "max_attempts": 2
      },
      "attempts": {
        "total": 0,
        "failed": 0
      },
      "order": 10,
      "overlap": []
    }
  ]
}`

// TestIndexJSONRoundTrip verifies JSON tags preserve the example structure.
func TestIndexJSONRoundTrip(t *testing.T) {
	var decoded Index
	if err := json.Unmarshal([]byte(exampleIndexJSON), &decoded); err != nil {
		t.Fatalf("unmarshal example index: %v", err)
	}

	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatalf("marshal example index: %v", err)
	}

	var roundTripped Index
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("unmarshal round-tripped index: %v", err)
	}

	if !reflect.DeepEqual(decoded, roundTripped) {
		t.Fatalf("round-tripped index does not match original")
	}
}

// TestIndexMissingFieldsZeroValues ensures missing fields fall back to zero values.
func TestIndexMissingFieldsZeroValues(t *testing.T) {
	const minimalJSON = `{
  "schema_version": 1,
  "digests": {
    "governator_md": "",
    "planning_docs": {}
  },
  "tasks": [
    {
      "id": "task-01",
      "path": "_governator/tasks/task-01.md",
      "state": "open",
      "role": "planner"
    }
  ]
}`

	var decoded Index
	if err := json.Unmarshal([]byte(minimalJSON), &decoded); err != nil {
		t.Fatalf("unmarshal minimal index: %v", err)
	}

	if len(decoded.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(decoded.Tasks))
	}

	task := decoded.Tasks[0]
	if task.Title != "" {
		t.Fatalf("expected empty title, got %q", task.Title)
	}
	if len(task.Dependencies) != 0 {
		t.Fatalf("expected empty dependencies, got %d", len(task.Dependencies))
	}
	if task.Retries.MaxAttempts != 0 {
		t.Fatalf("expected zero max attempts, got %d", task.Retries.MaxAttempts)
	}
	if task.Attempts.Total != 0 || task.Attempts.Failed != 0 {
		t.Fatalf("expected zero attempts, got total=%d failed=%d", task.Attempts.Total, task.Attempts.Failed)
	}
	if task.Order != 0 {
		t.Fatalf("expected zero order, got %d", task.Order)
	}
	if len(task.Overlap) != 0 {
		t.Fatalf("expected empty overlap, got %d", len(task.Overlap))
	}
}
