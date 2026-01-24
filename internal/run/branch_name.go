package run

import (
	"fmt"

	"github.com/cmtonkinson/governator/internal/index"
	"github.com/cmtonkinson/governator/internal/slug"
)

// TaskBranchName returns the canonical branch name for the provided task.
func TaskBranchName(task index.Task) string {
	base := fmt.Sprintf("task-%s", task.ID)
	if task.Title == "" {
		return base
	}
	slugified := slug.Slugify(task.Title)
	if slugified == "" {
		return base
	}
	return fmt.Sprintf("%s-%s", base, slugified)
}
