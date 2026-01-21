package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

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

	expected := Summary{
		Total:   1,
		Done:    0,
		Open:    1,
		Blocked: 0,
	}

	if summary != expected {
		t.Fatalf("GetSummary() = %+v, want %+v", summary, expected)
	}
}

// TestGetSummaryReportsMissingIndexError shows that removing the fixture index surface a clear error.
func TestGetSummaryReportsMissingIndexError(t *testing.T) {
	t.Parallel()

	repo := testrepos.New(t)
	repo.ApplyFixture(t, "single-task-flow")

	indexPath := filepath.Join(repo.Root, "_governator", "plan", "task-index.json")
	if err := os.Remove(indexPath); err != nil {
		t.Fatalf("remove fixture index: %v", err)
	}

	if _, err := GetSummary(repo.Root); err == nil {
		t.Fatalf("GetSummary() succeeded despite missing index")
	} else if !strings.Contains(err.Error(), "load task index") {
		t.Fatalf("unexpected error when index is missing: %v", err)
	}
}
