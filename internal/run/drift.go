// Package run provides helpers for governator run orchestration safeguards.
package run

import (
	"errors"
	"fmt"

	"github.com/cmtonkinson/governator/internal/digests"
	"github.com/cmtonkinson/governator/internal/index"
)

// ErrPlanningDrift indicates stored planning digests no longer match the repo state.
var ErrPlanningDrift = errors.New("planning drift detected")

// CheckPlanningDrift stops a run when planning artifacts have changed since planning.
func CheckPlanningDrift(repoRoot string, stored index.Digests) error {
	report, err := digests.Detect(repoRoot, stored)
	if err != nil {
		return fmt.Errorf("detect planning drift: %w", err)
	}
	if !report.HasDrift {
		return nil
	}

	message := fmt.Sprintf("%s\nNext steps: rerun `governator run` to regenerate planning artifacts and the task index.", report.Message)
	return fmt.Errorf("%w: %s", ErrPlanningDrift, message)
}
