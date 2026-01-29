package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
)

func TestSummaryString(t *testing.T) {
	empty := Summary{}
	if got := empty.String(); got != "supervisors=0\ntasks backlog=0 merged=0 in-progress=0" {
		t.Fatalf("empty summary string = %q", got)
	}

	withRows := Summary{
		Backlog:    1,
		Merged:     1,
		InProgress: 1,
		Rows: []statusRow{
			{id: "T-100", state: "triaged", pid: "1234", role: "builder", attrs: "blocked", title: "A task", order: 0},
		},
	}
	result := withRows.String()
	if !strings.Contains(result, "supervisors=0") {
		t.Fatalf("summary header missing supervisors: %q", result)
	}
	if !strings.Contains(result, "tasks backlog=1 merged=1 in-progress=1") {
		t.Fatalf("summary header missing counts: %q", result)
	}
	if !strings.Contains(result, "id") || !strings.Contains(result, "state") {
		t.Fatalf("table header missing: %q", result)
	}
}

func TestGetSummary(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "governator-status-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	stateDir := filepath.Join(tempDir, "_governator")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatalf("failed to create state dir: %v", err)
	}

	longTitle := strings.Repeat("x", titleMaxWidth+10)
	testIndex := index.Index{
		SchemaVersion: 1,
		Tasks: []index.Task{
			{ID: "T-backlog", Kind: index.TaskKindExecution, State: index.TaskStateBacklog},
			{ID: "T-triaged", Kind: index.TaskKindExecution, State: index.TaskStateTriaged, Role: "dev", AssignedRole: "dev"},
			{ID: "T-implemented", Kind: index.TaskKindExecution, State: index.TaskStateImplemented, Role: "dev"},
			{ID: "T-tested", Kind: index.TaskKindExecution, State: index.TaskStateTested, Role: "dev"},
			{ID: "T-reviewed", Kind: index.TaskKindExecution, State: index.TaskStateReviewed, Role: "dev"},
			{ID: "T-mergeable", Kind: index.TaskKindExecution, State: index.TaskStateMergeable, Role: "dev"},
			{ID: "T-merged", Kind: index.TaskKindExecution, State: index.TaskStateMerged, Role: "dev"},
			{ID: "T-blocked", Kind: index.TaskKindExecution, State: index.TaskStateBlocked, Role: "dev", BlockedReason: "blocked"},
			{ID: "T-conflict", Kind: index.TaskKindExecution, State: index.TaskStateConflict, Role: "dev", MergeConflict: true},
			{ID: "T-resolved", Kind: index.TaskKindExecution, State: index.TaskStateResolved, Role: "dev", Title: longTitle},
		},
	}

	indexPath := filepath.Join(tempDir, "_governator", "task-index.json")
	if err := index.Save(indexPath, testIndex); err != nil {
		t.Fatalf("failed to save test index: %v", err)
	}

	summary, err := GetSummary(tempDir)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	if summary.Backlog != 1 {
		t.Fatalf("expected 1 backlog task, got %d", summary.Backlog)
	}
	if summary.Merged != 1 {
		t.Fatalf("expected 1 merged task, got %d", summary.Merged)
	}
	if summary.InProgress != 8 {
		t.Fatalf("expected 8 in-progress tasks, got %d", summary.InProgress)
	}
	if len(summary.Rows) != summary.InProgress {
		t.Fatalf("expected %d rows, got %d", summary.InProgress, len(summary.Rows))
	}

	if summary.Rows[0].state != string(index.TaskStateTriaged) {
		t.Fatalf("expected first row state triaged, got %s", summary.Rows[0].state)
	}
	last := summary.Rows[len(summary.Rows)-1]
	if !strings.HasSuffix(last.title, "...") {
		t.Fatalf("expected truncated title, got %q", last.title)
	}

	foundAttrs := false
	for _, row := range summary.Rows {
		if row.id == "T-blocked" && row.attrs == "blocked" {
			foundAttrs = true
		}
	}
	if !foundAttrs {
		t.Fatalf("blocked attribute missing from rows: %+v", summary.Rows)
	}
}
