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

// PlanningDriftReport summarizes planning digest drift detected since the last refresh.
type PlanningDriftReport struct {
	HasDrift bool
	Details  []string
	Message  string
}

// DetectPlanningDrift reports planning digest drift relative to stored digests.
func DetectPlanningDrift(repoRoot string, stored index.Digests) (PlanningDriftReport, error) {
	report, err := digests.Detect(repoRoot, stored)
	if err != nil {
		return PlanningDriftReport{}, fmt.Errorf("detect planning drift: %w", err)
	}
	return PlanningDriftReport{
		HasDrift: report.HasDrift,
		Details:  report.Details,
		Message:  report.Message,
	}, nil
}

// CheckPlanningDrift stops a run when planning digests changed since planning.
func CheckPlanningDrift(repoRoot string, stored index.Digests) error {
	report, err := DetectPlanningDrift(repoRoot, stored)
	if err != nil {
		return fmt.Errorf("detect planning drift: %w", err)
	}
	if !report.HasDrift {
		return nil
	}

	message := fmt.Sprintf("%s\nNext steps: rerun `governator start` to regenerate planning artifacts and the task index.", report.Message)
	return fmt.Errorf("%w: %s", ErrPlanningDrift, message)
}
