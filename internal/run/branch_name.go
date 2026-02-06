package run

import (
	"github.com/cmtonkinson/governator/internal/index"
)

// TaskBranchName returns the canonical branch name for the provided task.
// The task ID already contains the slugified title and role (format: <id>-<slug>-<role>),
// so the branch name is just the task ID.
func TaskBranchName(task index.Task) string {
	return task.ID
}
