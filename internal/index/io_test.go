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
