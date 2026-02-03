package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/testrepos"
)

// TestGetSummaryUsesSingleTaskFixture exercises status summary calculation with the minimal single-task fixture.
func TestGetSummaryUsesSingleTaskFixture(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repo.ApplyFixture(t, "single-task-flow")

	summary, err := GetSummary(repo.Root)
	if err != nil {
		t.Fatalf("GetSummary() failed: %v", err)
	}

	if summary.Total != 1 {
		t.Fatalf("total = %d", summary.Total)
	}
	if summary.Backlog != 0 {
		t.Fatalf("backlog = %d", summary.Backlog)
	}
	if summary.Merged != 0 {
		t.Fatalf("merged = %d", summary.Merged)
	}
	if summary.InProgress != 1 {
		t.Fatalf("in-progress = %d", summary.InProgress)
	}
	if len(summary.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(summary.Rows))
	}
	if summary.Rows[0].state != string(index.TaskStateTriaged) {
		t.Fatalf("unexpected row state %q", summary.Rows[0].state)
	}
}

// TestGetSummaryReportsMissingIndexError shows that removing the fixture index surface a clear error.
func TestGetSummaryReportsMissingIndexError(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repo.ApplyFixture(t, "single-task-flow")

	indexPath := filepath.Join(repo.Root, "_governator", "index.json")
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove fixture index: %v", err)
	}

	if _, err := GetSummary(repo.Root); err == nil {
		t.Fatalf("GetSummary() succeeded despite missing index")
	} else if !strings.Contains(err.Error(), "load task index") {
		t.Fatalf("unexpected error when index is missing: %v", err)
	}
}
