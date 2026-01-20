// Tests for planner task index mapping helpers.
package planner

import (
	"reflect"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

// TestBuildIndexTasksMapsOrderAndOverlap ensures order and overlap are mapped.
func TestBuildIndexTasksMapsOrderAndOverlap(t *testing.T) {
	tasks := []PlannedTask{
		{
			ID:           "task-01",
			Title:        "Define Index",
			Summary:      "Define the index format.",
			Role:         "planner",
			Dependencies: []string{},
			Order:        10,
			Overlap:      []string{"planner", "index"},
			AcceptanceCriteria: []string{
				"Index format is documented.",
			},
			Tests: []string{
				"N/A",
			},
		},
		{
			ID:           "task-02",
			Title:        "Write Scheduler",
			Summary:      "Write scheduler ordering rules.",
			Role:         "planner",
			Dependencies: []string{"task-01"},
			Order:        20,
			Overlap:      []string{"scheduler"},
			AcceptanceCriteria: []string{
				"Scheduler rules are documented.",
			},
			Tests: []string{
				"N/A",
			},
		},
	}

	got, err := BuildIndexTasks(tasks, IndexTaskOptions{MaxAttempts: 2})
	if err != nil {
		t.Fatalf("build index tasks: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(got))
	}

	if got[0].Order != 10 {
		t.Fatalf("expected task-01 order 10, got %d", got[0].Order)
	}
	if !reflect.DeepEqual(got[0].Overlap, []string{"planner", "index"}) {
		t.Fatalf("expected task-01 overlap, got %#v", got[0].Overlap)
	}
	if got[0].Path != "_governator/tasks/task-01-define-index.md" {
		t.Fatalf("expected task-01 path, got %q", got[0].Path)
	}
	if got[0].State != index.TaskStateOpen {
		t.Fatalf("expected task-01 state open, got %q", got[0].State)
	}
	if got[0].Retries.MaxAttempts != 2 {
		t.Fatalf("expected task-01 retries 2, got %d", got[0].Retries.MaxAttempts)
	}
	if got[1].Order != 20 {
		t.Fatalf("expected task-02 order 20, got %d", got[1].Order)
	}
	if !reflect.DeepEqual(got[1].Overlap, []string{"scheduler"}) {
		t.Fatalf("expected task-02 overlap, got %#v", got[1].Overlap)
	}
}

// TestBuildIndexTasksDefaultsOverlap ensures missing overlap yields no conflicts.
func TestBuildIndexTasksDefaultsOverlap(t *testing.T) {
	tasks := []PlannedTask{
		{
			ID:           "task-03",
			Title:        "No Overlap",
			Summary:      "No overlap labels provided.",
			Role:         "planner",
			Dependencies: nil,
			Order:        30,
			Overlap:      nil,
			AcceptanceCriteria: []string{
				"Task uses default overlap.",
			},
			Tests: []string{
				"N/A",
			},
		},
	}

	got, err := BuildIndexTasks(tasks, IndexTaskOptions{})
	if err != nil {
		t.Fatalf("build index tasks: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 task, got %d", len(got))
	}
	if got[0].Overlap == nil || len(got[0].Overlap) != 0 {
		t.Fatalf("expected empty overlap, got %#v", got[0].Overlap)
	}
	if got[0].Dependencies == nil || len(got[0].Dependencies) != 0 {
		t.Fatalf("expected empty dependencies, got %#v", got[0].Dependencies)
	}
}
