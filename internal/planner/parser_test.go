// Package planner tests planner output parsing.
package planner

import (
	"strings"
	"testing"
)

const plannerOutputFixture = `{
  "schema_version": 1,
  "kind": "planner_output",
  "architecture_baseline": {
    "schema_version": 1,
    "kind": "architecture_baseline",
    "mode": "synthesis",
    "summary": "Single CLI binary with a minimal planning pipeline.",
    "sources": [
      "_governator/architecture/context.md",
      "GOVERNATOR.md"
    ]
  },
  "roadmap": {
    "schema_version": 1,
    "kind": "roadmap_decomposition",
    "depth_policy": "epic->task",
    "width_policy": "1-3 days",
    "items": [
      {
        "id": "epic-01",
        "title": "Planner contract and parsing",
        "type": "epic",
        "order": 10
      }
    ]
  },
  "tasks": {
    "schema_version": 1,
    "kind": "task_generation",
    "tasks": [
      {
        "id": "task-11",
        "title": "Implement planner output parser",
        "summary": "Parse planner JSON output into internal task models.",
        "role": "engineer",
        "dependencies": [],
        "order": 20,
        "overlap": [
          "planner",
          "index"
        ],
        "acceptance_criteria": [
          "Parser handles all planner output fields."
        ],
        "tests": [
          "Unit tests cover valid and invalid output."
        ]
      }
    ]
  }
}`

func TestParsePlannerOutputHappyPath(t *testing.T) {
	output, err := ParsePlannerOutput([]byte(plannerOutputFixture))
	if err != nil {
		t.Fatalf("expected planner output, got error: %v", err)
	}
	if output.Kind != plannerOutputKind {
		t.Fatalf("expected kind %q, got %q", plannerOutputKind, output.Kind)
	}
	if output.GapAnalysis != nil {
		t.Fatal("expected gap analysis to be absent")
	}
	if len(output.Tasks.Tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(output.Tasks.Tasks))
	}
	if output.Tasks.Tasks[0].ID != "task-11" {
		t.Fatalf("expected task id task-11, got %q", output.Tasks.Tasks[0].ID)
	}
	if output.ArchitectureBaseline.Components == nil {
		t.Fatal("expected components to default to an empty slice")
	}
	if len(output.ArchitectureBaseline.Components) != 0 {
		t.Fatal("expected components to be empty")
	}
	if output.Roadmap.Items[0].Overlap == nil {
		t.Fatal("expected roadmap overlap to default to an empty slice")
	}
	if len(output.Roadmap.Items[0].Overlap) != 0 {
		t.Fatal("expected roadmap overlap to be empty")
	}
}

func TestParsePlannerOutputMalformedJSON(t *testing.T) {
	_, err := ParsePlannerOutput([]byte("{"))
	if err == nil {
		t.Fatal("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "decode planner output JSON") {
		t.Fatalf("expected decode error, got %v", err)
	}
}
