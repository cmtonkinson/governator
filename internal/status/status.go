// Package status provides task index status reporting.
package status

import (
	"fmt"
	"path/filepath"

	"github.com/cmtonkinson/governator/internal/index"
)

// Summary represents task counts by state.
type Summary struct {
	Total   int
	Done    int
	Open    int
	Blocked int
}

// String returns the formatted status output as expected by the CLI contract.
// Format: "tasks total=<n> done=<n> open=<n> blocked=<n>"
func (s Summary) String() string {
	return fmt.Sprintf("tasks total=%d done=%d open=%d blocked=%d", s.Total, s.Done, s.Open, s.Blocked)
}

// GetSummary reads the task index and returns a summary of task states.
func GetSummary(repoRoot string) (Summary, error) {
	indexPath := filepath.Join(repoRoot, "_governator", "task-index.json")

	idx, err := index.Load(indexPath)
	if err != nil {
		return Summary{}, fmt.Errorf("load task index: %w", err)
	}

	summary := Summary{
		Total: len(idx.Tasks),
	}

	for _, task := range idx.Tasks {
		switch task.State {
		case index.TaskStateDone:
			summary.Done++
		case index.TaskStateOpen:
			summary.Open++
		case index.TaskStateBlocked, index.TaskStateConflict:
			summary.Blocked++
		case index.TaskStateWorked, index.TaskStateTested, index.TaskStateResolved:
			// These are considered "in progress" but not blocked, count as open
			summary.Open++
		}
	}

	return summary, nil
}
