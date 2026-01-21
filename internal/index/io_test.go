// Tests for task index IO helpers.
package index

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestIndexLoadSaveRoundTrip verifies loading and saving persists updates.
func TestIndexLoadSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join(dir, "index.json")

	if err := os.WriteFile(inPath, []byte(exampleIndexJSON), 0o644); err != nil {
		t.Fatalf("write input index: %v", err)
	}

	idx, err := Load(inPath)
	if err != nil {
		t.Fatalf("load index: %v", err)
	}

	idx.Tasks[0].State = TaskStateWorked

	outPath := filepath.Join(dir, "index-out.json")
	if err := Save(outPath, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	reloaded, err := Load(outPath)
	if err != nil {
		t.Fatalf("reload index: %v", err)
	}

	if got := reloaded.Tasks[0].State; got != TaskStateWorked {
		t.Fatalf("expected state %q, got %q", TaskStateWorked, got)
	}
}

// TestIndexSaveDeterministicPlanningDocs ensures planning docs map keys sort.
func TestIndexSaveDeterministicPlanningDocs(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Digests: Digests{
			GovernatorMD: "",
			PlanningDocs: map[string]string{
				"b": "second",
				"a": "first",
			},
		},
	}

	path := filepath.Join(t.TempDir(), "index.json")
	if err := Save(path, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read index output: %v", err)
	}

	content := string(data)
	patternA := regexp.MustCompile(`"a"\s*:\s*"first"`)
	patternB := regexp.MustCompile(`"b"\s*:\s*"second"`)
	aLoc := patternA.FindStringIndex(content)
	bLoc := patternB.FindStringIndex(content)
	if aLoc == nil || bLoc == nil {
		t.Fatalf("expected planning docs entries in output, got %s", content)
	}
	if aLoc[0] > bLoc[0] {
		t.Fatalf("expected planning docs keys sorted, got %s", content)
	}
}

// TestIndexSaveDeterministicTasksAndDeps ensures tasks and lists are sorted.
func TestIndexSaveDeterministicTasksAndDeps(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Digests: Digests{
			GovernatorMD: "",
			PlanningDocs: map[string]string{},
		},
		Tasks: []Task{
			{
				ID:           "task-b",
				Path:         "_governator/tasks/task-b.md",
				State:        TaskStateOpen,
				Role:         "worker",
				Dependencies: []string{"dep-b", "dep-a"},
				Overlap:      []string{"overlap-b", "overlap-a"},
				Order:        20,
			},
			{
				ID:           "task-a",
				Path:         "_governator/tasks/task-a.md",
				State:        TaskStateOpen,
				Role:         "worker",
				Dependencies: []string{"dep-d", "dep-c"},
				Overlap:      []string{"overlap-d", "overlap-c"},
				Order:        10,
			},
		},
	}

	path := filepath.Join(t.TempDir(), "index.json")
	if err := Save(path, idx); err != nil {
		t.Fatalf("save index: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read index output: %v", err)
	}

	content := string(data)
	taskALoc := strings.Index(content, `"id": "task-a"`)
	taskBLoc := strings.Index(content, `"id": "task-b"`)
	if taskALoc == -1 || taskBLoc == -1 {
		t.Fatalf("expected task ids in output, got %s", content)
	}
	if taskALoc > taskBLoc {
		t.Fatalf("expected task ordering by order/id, got %s", content)
	}

	depPattern := regexp.MustCompile(`"dependencies"\s*:\s*\[\s*"dep-a"\s*,\s*"dep-b"\s*\]`)
	if !depPattern.MatchString(content) {
		t.Fatalf("expected sorted dependencies for task-b, got %s", content)
	}

	overlapPattern := regexp.MustCompile(`"overlap"\s*:\s*\[\s*"overlap-a"\s*,\s*"overlap-b"\s*\]`)
	if !overlapPattern.MatchString(content) {
		t.Fatalf("expected sorted overlap for task-b, got %s", content)
	}
}

// TestIndexSaveRoundTripDeterministic verifies save-load-save stability.
func TestIndexSaveRoundTripDeterministic(t *testing.T) {
	idx := Index{
		SchemaVersion: 1,
		Digests: Digests{
			GovernatorMD: "sha256:abc",
			PlanningDocs: map[string]string{
				"plan-b": "sha256:def",
				"plan-a": "sha256:ghi",
			},
		},
		Tasks: []Task{
			{
				ID:           "task-b",
				Path:         "_governator/tasks/task-b.md",
				State:        TaskStateOpen,
				Role:         "worker",
				Dependencies: []string{"dep-b", "dep-a"},
				Order:        2,
			},
			{
				ID:           "task-a",
				Path:         "_governator/tasks/task-a.md",
				State:        TaskStateOpen,
				Role:         "worker",
				Dependencies: []string{"dep-d", "dep-c"},
				Order:        1,
			},
		},
	}

	dir := t.TempDir()
	first := filepath.Join(dir, "index-first.json")
	second := filepath.Join(dir, "index-second.json")

	if err := Save(first, idx); err != nil {
		t.Fatalf("save first index: %v", err)
	}

	loaded, err := Load(first)
	if err != nil {
		t.Fatalf("load first index: %v", err)
	}

	if err := Save(second, loaded); err != nil {
		t.Fatalf("save second index: %v", err)
	}

	firstData, err := os.ReadFile(first)
	if err != nil {
		t.Fatalf("read first index: %v", err)
	}
	secondData, err := os.ReadFile(second)
	if err != nil {
		t.Fatalf("read second index: %v", err)
	}

	if string(firstData) != string(secondData) {
		t.Fatalf("expected deterministic output, got first=%s second=%s", firstData, secondData)
	}
}

// TestIndexLoadMalformedJSON ensures malformed input returns an actionable error.
func TestIndexLoadMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatalf("write malformed index: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected error for malformed JSON")
	}
	if !strings.Contains(err.Error(), "read task index") {
		t.Fatalf("expected error to mention task index read, got %q", err.Error())
	}
}

// TestIndexLoadTrailingContentError verifies trailing data produces a parsing failure.
func TestIndexLoadTrailingContentError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "index.json")
	content := `{"schema_version":1,"digests":{"governator_md":"","planning_docs":{}},"tasks":[]}`
	if err := os.WriteFile(path, []byte(content+"\n{}"), 0o644); err != nil {
		t.Fatalf("write index with trailing content: %v", err)
	}

	_, err := Load(path)
	if err == nil {
		t.Fatalf("expected error for trailing content")
	}
	if !strings.Contains(err.Error(), "invalid trailing content after JSON object") {
		t.Fatalf("expected trailing content error, got %q", err.Error())
	}
}

// TestEncodePlanningDocsDeterministicOrder ensures planning doc keys are serialized in sorted order.
func TestEncodePlanningDocsDeterministicOrder(t *testing.T) {
	planningDocs := map[string]string{
		"z": "last",
		"a": "first",
		"m": "middle",
	}

	encoded, err := encodePlanningDocs(planningDocs)
	if err != nil {
		t.Fatalf("encode planning docs: %v", err)
	}

	const want = `{"a":"first","m":"middle","z":"last"}`
	if string(encoded) != want {
		t.Fatalf("planning docs encoding not deterministic, got %s", encoded)
	}
}

// TestNormalizeTasksForWriteTieBreakers covers deterministic ordering when tasks tie on order and id.
func TestNormalizeTasksForWriteTieBreakers(t *testing.T) {
	unordered := []Task{
		{
			ID:           "task-b",
			Path:         "_governator/tasks/task-b.md",
			Role:         "worker",
			Order:        1,
			Dependencies: []string{"dep-b", "dep-a"},
			Overlap:      []string{"overlap-b", "overlap-a"},
		},
		{
			ID:           "task-a",
			Path:         "_governator/tasks/task-z.md",
			Role:         "worker",
			Order:        1,
			Dependencies: []string{"dep-b", "dep-a"},
			Overlap:      []string{"overlap-b", "overlap-a"},
		},
		{
			ID:           "task-a",
			Path:         "_governator/tasks/task-a.md",
			Role:         "worker",
			Order:        1,
			Dependencies: []string{"dep-b", "dep-a"},
			Overlap:      []string{"overlap-b", "overlap-a"},
		},
		{
			ID:           "task-a",
			Path:         "_governator/tasks/task-a.md",
			Role:         "architect",
			Order:        1,
			Dependencies: []string{"dep-b", "dep-a"},
			Overlap:      []string{"overlap-b", "overlap-a"},
		},
	}

	normalized := normalizeTasksForWrite(unordered)
	if len(normalized) != len(unordered) {
		t.Fatalf("unexpected normalized task count, got %d", len(normalized))
	}

	want := []struct {
		id   string
		path string
		role string
	}{
		{"task-a", "_governator/tasks/task-a.md", "architect"},
		{"task-a", "_governator/tasks/task-a.md", "worker"},
		{"task-a", "_governator/tasks/task-z.md", "worker"},
		{"task-b", "_governator/tasks/task-b.md", "worker"},
	}

	for i, expected := range want {
		task := normalized[i]
		if task.ID != expected.id || task.Path != expected.path || string(task.Role) != expected.role {
			t.Fatalf("normalized[%d] = (%s,%s,%s), expected (%s,%s,%s)", i, task.ID, task.Path, task.Role, expected.id, expected.path, expected.role)
		}

		if len(task.Dependencies) != 2 || task.Dependencies[0] != "dep-a" || task.Dependencies[1] != "dep-b" {
			t.Fatalf("expected dependencies sorted for task %s, got %v", task.ID, task.Dependencies)
		}
		if len(task.Overlap) != 2 || task.Overlap[0] != "overlap-a" || task.Overlap[1] != "overlap-b" {
			t.Fatalf("expected overlap sorted for task %s, got %v", task.ID, task.Overlap)
		}
	}
}
