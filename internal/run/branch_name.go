package run

import (
	"fmt"

	"github.com/cmtonkinson/governator/internal/index"
)

// TaskBranchName returns the canonical branch name for the provided task.
// The task ID already contains the slugified title and role (format: <id>-<slug>-<role>),
// so we just prefix it with "task-" to create the branch name.
func TaskBranchName(task index.Task) string {
	return fmt.Sprintf("task-%s", task.ID)
}
